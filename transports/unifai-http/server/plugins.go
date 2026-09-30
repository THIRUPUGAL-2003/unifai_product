package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"

	"github.com/unifai/unifai/core/schemas"
	"github.com/unifai/unifai/framework/configstore"
	"github.com/unifai/unifai/plugins/compat"
	"github.com/unifai/unifai/plugins/connectors"
	"github.com/unifai/unifai/plugins/governance"
	"github.com/unifai/unifai/plugins/logging"
	"github.com/unifai/unifai/plugins/maxim"
	"github.com/unifai/unifai/plugins/modelcatalogresolver"
	"github.com/unifai/unifai/plugins/otel"
	"github.com/unifai/unifai/plugins/prompts"
	"github.com/unifai/unifai/plugins/semanticcache"
	"github.com/unifai/unifai/plugins/telemetry"
	"github.com/unifai/unifai/plugins/guardrails"
	"github.com/unifai/unifai/transports/unifai-http/handlers"
	"github.com/unifai/unifai/transports/unifai-http/lib"
)

// InferPluginTypes determines which interface types a plugin implements
func InferPluginTypes(plugin schemas.BasePlugin) []schemas.PluginType {
	var types []schemas.PluginType
	if _, ok := plugin.(schemas.LLMPlugin); ok {
		types = append(types, schemas.PluginTypeLLM)
	}
	if _, ok := plugin.(schemas.MCPPlugin); ok {
		types = append(types, schemas.PluginTypeMCP)
	}
	if _, ok := plugin.(schemas.HTTPTransportPlugin); ok {
		types = append(types, schemas.PluginTypeHTTP)
	}
	return types
}

// Single-plugin methods used plugin create/update

// InstantiatePlugin creates a plugin instance but does NOT register it
// Registration is done separately via Config.RegisterPlugin()
func InstantiatePlugin(ctx context.Context, name string, path *string, pluginConfig any, unifaiConfig *lib.Config) (schemas.BasePlugin, error) {
	// Custom plugin (has path)
	if path != nil {
		return loadCustomPlugin(ctx, path, pluginConfig, unifaiConfig)
	}

	// Built-in plugin (by name)
	return loadBuiltinPlugin(ctx, name, pluginConfig, unifaiConfig)
}

// loadBuiltinPlugin instantiates a built-in plugin by name
func loadBuiltinPlugin(ctx context.Context, name string, pluginConfig any, unifaiConfig *lib.Config) (schemas.BasePlugin, error) {
	switch name {
	case telemetry.PluginName:
		telConfig := &telemetry.Config{
			CustomLabels: unifaiConfig.ClientConfig.PrometheusLabels,
		}
		// Merge persisted config if provided.
		if pluginConfig != nil {
			extraConfig, err := MarshalPluginConfig[telemetry.Config](pluginConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal telemetry plugin config: %w", err)
			}
			if extraConfig != nil {
				if extraConfig.PushGateway != nil {
					telConfig.PushGateway = extraConfig.PushGateway
				}
				if extraConfig.MetricsEnabled != nil {
					telConfig.MetricsEnabled = extraConfig.MetricsEnabled
				}
			}
		}
		return telemetry.Init(telConfig, unifaiConfig.ModelCatalog, logger)

	case prompts.PluginName:
		return prompts.Init(ctx, unifaiConfig.ConfigStore, logger)

	case logging.PluginName:
		loggingConfig, err := MarshalPluginConfig[logging.Config](pluginConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal logging plugin config: %w", err)
		}
		return logging.Init(ctx, loggingConfig, logger, unifaiConfig.LogsStore,
			unifaiConfig.ModelCatalog, unifaiConfig.MCPCatalog)

	case governance.PluginName:
		governanceConfig, err := MarshalPluginConfig[governance.Config](pluginConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal governance plugin config: %w", err)
		}
		inMemoryStore := &GovernanceInMemoryStore{Config: unifaiConfig}
		return governance.Init(ctx, governanceConfig, logger, unifaiConfig.ConfigStore,
			unifaiConfig.GovernanceConfig, unifaiConfig.ModelCatalog,
			unifaiConfig.MCPCatalog, inMemoryStore)

	case maxim.PluginName:
		maximConfig, err := MarshalPluginConfig[maxim.Config](pluginConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal maxim plugin config: %w", err)
		}
		return maxim.Init(maximConfig, logger)

	case semanticcache.PluginName:
		semanticConfig, err := MarshalPluginConfig[semanticcache.Config](pluginConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal semantic cache plugin config: %w", err)
		}
		return semanticcache.Init(ctx, semanticConfig, logger, unifaiConfig.VectorStore)

	case otel.PluginName:
		otelConfig, err := MarshalPluginConfig[otel.Config](pluginConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal otel plugin config: %w", err)
		}
		return otel.Init(ctx, otelConfig, logger, unifaiConfig.ModelCatalog, handlers.GetVersion())

	case compat.PluginName:
		compatConfig, err := MarshalPluginConfig[compat.Config](pluginConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal compat plugin config: %w", err)
		}
		return compat.Init(*compatConfig, logger, unifaiConfig.ModelCatalog)

	case guardrails.PluginName:
		guardrailsConfig := &guardrails.Config{}
		// We have to copy the lib config to the local plugin config.
		// Since we want to avoid import cycles, we marshal and unmarshal.
		if unifaiConfig.GuardrailsConfig != nil {
			bytes, err := json.Marshal(unifaiConfig.GuardrailsConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal guardrails config: %w", err)
			}
			if err := json.Unmarshal(bytes, guardrailsConfig); err != nil {
				return nil, fmt.Errorf("failed to unmarshal guardrails config: %w", err)
			}
		}
		return guardrails.Init(ctx, guardrailsConfig, logger)

	case modelcatalogresolver.PluginName:
		return modelcatalogresolver.Init(unifaiConfig.ModelCatalog, logger)

	case connectors.PluginName:
		return connectors.Init()

	default:
		return nil, fmt.Errorf("unknown built-in plugin: %s", name)
	}
}

// loadCustomPlugin loads a plugin from a shared object file
func loadCustomPlugin(ctx context.Context, path *string, pluginConfig any, unifaiConfig *lib.Config) (schemas.BasePlugin, error) {
	logger.Info("loading custom plugin from path %s", *path)

	plugin, err := unifaiConfig.PluginLoader.LoadPlugin(*path, pluginConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to load custom plugin: %w", err)
	}
	return plugin, nil
}

// LoadPlugins loads the plugins for the server.
func (s *UnifAIHTTPServer) LoadPlugins(ctx context.Context) error {
	// Load built-in plugins first (order matters)
	if err := s.loadBuiltinPlugins(ctx); err != nil {
		return err
	}
	// Load custom plugins from config
	if err := s.loadCustomPlugins(ctx); err != nil {
		return err
	}
	// Sort all plugins by placement group and order
	s.Config.SortAndRebuildPlugins()
	return nil
}

// getPluginConfig retrieves a plugin's config from PluginConfigs by name
func (s *UnifAIHTTPServer) getPluginConfig(name string) *schemas.PluginConfig {
	for _, cfg := range s.Config.PluginConfigs {
		if cfg.Name == name {
			return cfg
		}
	}
	return nil
}

// loadBuiltinPlugins loads required built-in plugins in specific order
func (s *UnifAIHTTPServer) loadBuiltinPlugins(ctx context.Context) error {
	builtinPlacement := schemas.Ptr(schemas.PluginPlacementBuiltin)

	// 1. Telemetry (always first - tracks everything).
	// Default-on: absent PluginConfig entry is treated as enabled, matching pre-#3269 behavior
	// so upgraders don't silently lose /metrics. Only an explicit Enabled=false disables it.
	telemetryPluginConfig := s.getPluginConfig(telemetry.PluginName)
	var pluginConfig any
	if telemetryPluginConfig != nil {
		pluginConfig = telemetryPluginConfig.Config
	}
	if telemetryPluginConfig == nil || telemetryPluginConfig.Enabled {
		s.registerPluginWithStatus(ctx, telemetry.PluginName, nil, pluginConfig, false)
	} else {
		s.markPluginDisabled(telemetry.PluginName)
	}
	s.Config.SetPluginOrderInfo(telemetry.PluginName, builtinPlacement, schemas.Ptr(1))

	// 2. Prompts (requires config store for prompt repository; disabled in enterprise)
	if s.Config.ConfigStore != nil && ctx.Value(schemas.UnifAIContextKeyIsEnterprise) == nil {
		s.registerPluginWithStatus(ctx, prompts.PluginName, nil, nil, false)
	} else {
		s.markPluginDisabled(prompts.PluginName)
	}
	s.Config.SetPluginOrderInfo(prompts.PluginName, builtinPlacement, schemas.Ptr(2))

	// 3. Logging (if enabled)
	if (s.Config.ClientConfig.EnableLogging == nil || *s.Config.ClientConfig.EnableLogging) && s.Config.LogsStore != nil {
		config := &logging.Config{
			DisableContentLogging: &s.Config.ClientConfig.DisableContentLogging,
			LoggingHeaders:        &s.Config.ClientConfig.LoggingHeaders,
		}
		if s.Config.LogsStoreConfig != nil {
			config.Writer = s.Config.LogsStoreConfig.Writer
		}
		s.registerPluginWithStatus(ctx, logging.PluginName, nil, config, false)
	} else {
		s.markPluginDisabled(logging.PluginName)
	}
	s.Config.SetPluginOrderInfo(logging.PluginName, builtinPlacement, schemas.Ptr(3))

	// 4. Governance (if enabled and not enterprise)
	if ctx.Value(schemas.UnifAIContextKeyIsEnterprise) == nil {
		config := &governance.Config{
			IsVkMandatory:         &s.Config.ClientConfig.EnforceAuthOnInference,
			RequiredHeaders:       &s.Config.ClientConfig.RequiredHeaders,
			DisableAutoToolInject: &s.Config.ClientConfig.MCPDisableAutoToolInject,
			RoutingChainMaxDepth:  &s.Config.ClientConfig.RoutingChainMaxDepth,
		}
		s.registerPluginWithStatus(ctx, governance.PluginName, nil, config, false)
	} else {
		s.markPluginDisabled(governance.PluginName)
	}
	s.Config.SetPluginOrderInfo(governance.PluginName, builtinPlacement, schemas.Ptr(4))

	// 5. OTEL (if configured in PluginConfigs)
	otelConfig := s.getPluginConfig(otel.PluginName)
	if otelConfig != nil && otelConfig.Enabled {
		s.registerPluginWithStatus(ctx, otel.PluginName, nil, otelConfig.Config, false)
	} else {
		s.markPluginDisabled(otel.PluginName)
	}
	s.Config.SetPluginOrderInfo(otel.PluginName, builtinPlacement, schemas.Ptr(5))

	// 6. Semantic Cache (if configured in PluginConfigs)
	semanticCacheConfig := s.getPluginConfig(semanticcache.PluginName)
	if semanticCacheConfig != nil && semanticCacheConfig.Enabled {
		s.registerPluginWithStatus(ctx, semanticcache.PluginName, nil, semanticCacheConfig.Config, false)
	} else {
		s.markPluginDisabled(semanticcache.PluginName)
	}
	s.Config.SetPluginOrderInfo(semanticcache.PluginName, builtinPlacement, schemas.Ptr(6))

	// 7. Compat (if any compat feature is enabled in ClientConfig)
	cc := s.Config.ClientConfig.Compat
	compatCfg := &compat.Config{
		ConvertTextToChat:      cc.ConvertTextToChat,
		ConvertChatToResponses: cc.ConvertChatToResponses,
		ShouldDropParams:       cc.ShouldDropParams,
		ShouldConvertParams:    cc.ShouldConvertParams,
	}
	s.registerPluginWithStatus(ctx, compat.PluginName, nil, compatCfg, false)
	s.Config.SetPluginOrderInfo(compat.PluginName, builtinPlacement, schemas.Ptr(7))

	// 8. Maxim (if configured in PluginConfigs)
	maximConfig := s.getPluginConfig(maxim.PluginName)
	if maximConfig != nil && maximConfig.Enabled {
		s.registerPluginWithStatus(ctx, maxim.PluginName, nil, maximConfig.Config, false)
	} else {
		s.markPluginDisabled(maxim.PluginName)
	}
	s.Config.SetPluginOrderInfo(maxim.PluginName, builtinPlacement, schemas.Ptr(8))

	// 9. Guardrails (always register; config may be empty until UI/API updates it)
	s.registerPluginWithStatus(ctx, guardrails.PluginName, nil, nil, false)
	s.Config.SetPluginOrderInfo(guardrails.PluginName, builtinPlacement, schemas.Ptr(9))

	// 10. ModelCatalogResolver (last routing layer — fills req.Provider from catalog only when
	// no earlier routing plugin (governance routing rules, governance VK LB, enterprise LB)
	// already set one. CEL rules can still match on provider == "" because this runs last.
	// Requires a model catalog; only register when one is configured.
	if s.Config.ModelCatalog != nil {
		s.registerPluginWithStatus(ctx, modelcatalogresolver.PluginName, nil, nil, false)
	} else {
		s.markPluginDisabled(modelcatalogresolver.PluginName)
	}
	// Place it in post_builtin with a max order so it runs after every other routing plugin,
	// including post_builtin ones like the enterprise load balancer (which would otherwise run
	// after this builtin and never get a chance to pick the provider first).
	s.Config.SetPluginOrderInfo(modelcatalogresolver.PluginName, schemas.Ptr(schemas.PluginPlacementPostBuiltin), schemas.Ptr(math.MaxInt))

	// 11. Workspace data connectors (Datadog, Kafka, BigQuery, Pub/Sub) — export traces when enabled.
	if _, ok := configstore.AsWorkspaceStore(s.Config.ConfigStore); ok {
		s.registerPluginWithStatus(ctx, connectors.PluginName, nil, nil, false)
	} else {
		s.markPluginDisabled(connectors.PluginName)
	}
	s.Config.SetPluginOrderInfo(connectors.PluginName, builtinPlacement, schemas.Ptr(10))

	return nil
}

// loadCustomPlugins loads plugins from PluginConfigs
func (s *UnifAIHTTPServer) loadCustomPlugins(ctx context.Context) error {
	for _, cfg := range s.Config.PluginConfigs {
		// Skip built-ins (already loaded)
		if lib.IsBuiltinPlugin(cfg.Name) {
			continue
		}
		// Handle disabled plugins
		if !cfg.Enabled {
			// For custom plugins with a path, verify to get the real plugin name
			if cfg.Path != nil {
				pluginName, err := s.Config.PluginLoader.VerifyBasePlugin(*cfg.Path)
				if err != nil {
					logger.Error("failed to verify disabled plugin %s: %v", cfg.Name, err)
					continue
				}
				// Store plugin status without instantiating (no Init() call, no resource usage)
				// Note: We can't determine types without instantiating, so pass empty slice
				s.Config.UpdatePluginOverallStatus(pluginName, cfg.Name, schemas.PluginStatusDisabled,
					[]string{fmt.Sprintf("plugin %s is disabled", cfg.Name)}, []schemas.PluginType{})
			} else {
				// Built-in plugin - use cfg.Name directly
				s.Config.UpdatePluginOverallStatus(cfg.Name, cfg.Name, schemas.PluginStatusDisabled,
					[]string{fmt.Sprintf("plugin %s is disabled", cfg.Name)}, []schemas.PluginType{})
			}
			continue
		}

		// Plugin is enabled - instantiate it
		plugin, err := InstantiatePlugin(ctx, cfg.Name, cfg.Path, cfg.Config, s.Config)
		if err != nil {
			// Skip enterprise plugins silently
			if slices.Contains(enterprisePlugins, cfg.Name) {
				continue
			}
			logger.Error("failed to load plugin %s: %v", cfg.Name, err)
			// Use cfg.Name since plugin may be nil when InstantiatePlugin returns an error
			s.Config.UpdatePluginOverallStatus(cfg.Name, cfg.Name, schemas.PluginStatusError,
				[]string{fmt.Sprintf("error loading plugin %s: %v", cfg.Name, err)}, []schemas.PluginType{})
			continue
		}

		// Ensure plugin is not nil before using it (defensive check)
		if plugin == nil {
			logger.Error("plugin %s instantiated but returned nil", cfg.Name)
			s.Config.UpdatePluginOverallStatus(cfg.Name, cfg.Name, schemas.PluginStatusError,
				[]string{fmt.Sprintf("plugin %s instantiated but returned nil", cfg.Name)}, []schemas.PluginType{})
			continue
		}

		// Register enabled plugin and mark as active
		s.Config.ReloadPlugin(plugin)
		s.Config.SetPluginOrderInfo(plugin.GetName(), cfg.Placement, cfg.Order)
		s.Config.UpdatePluginOverallStatus(plugin.GetName(), cfg.Name, schemas.PluginStatusActive,
			[]string{fmt.Sprintf("plugin %s initialized successfully", cfg.Name)}, InferPluginTypes(plugin))
	}
	return nil
}
