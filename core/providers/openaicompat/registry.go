package openaicompat

import "github.com/unifai/unifai/core/schemas"

// Spec describes a thin OpenAI-compatible provider.
type Spec struct {
	Key            schemas.ModelProvider
	DefaultBaseURL string
	// PathPrefix is inserted before resource paths (e.g. "v1", "v2", or "").
	PathPrefix string
	Embedding  bool
}

// Registry maps built-in OpenAI-compatible providers to default endpoints.
var Registry = map[schemas.ModelProvider]Spec{
	schemas.Together: {Key: schemas.Together, DefaultBaseURL: "https://api.together.xyz", PathPrefix: "v1", Embedding: false},
	schemas.Siliconflow: {Key: schemas.Siliconflow, DefaultBaseURL: "https://api.siliconflow.cn", PathPrefix: "v1", Embedding: false},
	schemas.Moonshot: {Key: schemas.Moonshot, DefaultBaseURL: "https://api.moonshot.ai", PathPrefix: "v1", Embedding: false},
	schemas.Minimax: {Key: schemas.Minimax, DefaultBaseURL: "https://api.minimax.io", PathPrefix: "v1", Embedding: false},
	schemas.Sambanova: {Key: schemas.Sambanova, DefaultBaseURL: "https://api.sambanova.ai", PathPrefix: "v1", Embedding: false},
	schemas.Deepinfra: {Key: schemas.Deepinfra, DefaultBaseURL: "https://api.deepinfra.com/v1/openai", PathPrefix: "", Embedding: false},
	schemas.Novita: {Key: schemas.Novita, DefaultBaseURL: "https://api.novita.ai/v3/openai", PathPrefix: "", Embedding: false},
	schemas.Nvidia: {Key: schemas.Nvidia, DefaultBaseURL: "https://integrate.api.nvidia.com", PathPrefix: "v1", Embedding: false},
	schemas.Hyperbolic: {Key: schemas.Hyperbolic, DefaultBaseURL: "https://api.hyperbolic.xyz", PathPrefix: "v1", Embedding: false},
	schemas.Portkey: {Key: schemas.Portkey, DefaultBaseURL: "https://api.portkey.ai", PathPrefix: "v1", Embedding: false},
	schemas.Dashscope: {Key: schemas.Dashscope, DefaultBaseURL: "https://dashscope-intl.aliyuncs.com/compatible-mode", PathPrefix: "v1", Embedding: false},
	schemas.Zhipu: {Key: schemas.Zhipu, DefaultBaseURL: "https://open.bigmodel.cn/api/paas/v4", PathPrefix: "", Embedding: false},
	schemas.Baichuan: {Key: schemas.Baichuan, DefaultBaseURL: "https://api.baichuan-ai.com", PathPrefix: "v1", Embedding: false},
	schemas.Stepfun: {Key: schemas.Stepfun, DefaultBaseURL: "https://api.stepfun.com", PathPrefix: "v1", Embedding: false},
	schemas.Upstage: {Key: schemas.Upstage, DefaultBaseURL: "https://api.upstage.ai/v1/solar", PathPrefix: "", Embedding: false},
	schemas.Ai21: {Key: schemas.Ai21, DefaultBaseURL: "https://api.ai21.com/studio", PathPrefix: "v1", Embedding: false},
	schemas.Sakana: {Key: schemas.Sakana, DefaultBaseURL: "https://api.sakana.ai", PathPrefix: "v1", Embedding: false},
	schemas.Baseten: {Key: schemas.Baseten, DefaultBaseURL: "https://inference.baseten.co", PathPrefix: "v1", Embedding: false},
	schemas.Anyscale: {Key: schemas.Anyscale, DefaultBaseURL: "https://api.endpoints.anyscale.com", PathPrefix: "v1", Embedding: false},
	schemas.Friendli: {Key: schemas.Friendli, DefaultBaseURL: "https://api.friendli.ai/serverless", PathPrefix: "v1", Embedding: false},
	schemas.Modelscope: {Key: schemas.Modelscope, DefaultBaseURL: "https://api-inference.modelscope.cn", PathPrefix: "v1", Embedding: false},
	schemas.Hunyuan: {Key: schemas.Hunyuan, DefaultBaseURL: "https://api.hunyuan.cloud.tencent.com", PathPrefix: "v1", Embedding: false},
	schemas.Qianfan: {Key: schemas.Qianfan, DefaultBaseURL: "https://qianfan.baidubce.com", PathPrefix: "v2", Embedding: false},
	schemas.Ark: {Key: schemas.Ark, DefaultBaseURL: "https://ark.cn-beijing.volces.com/api/v3", PathPrefix: "", Embedding: false},
	schemas.Sarvam: {Key: schemas.Sarvam, DefaultBaseURL: "https://api.sarvam.ai", PathPrefix: "v1", Embedding: false},
	schemas.Krutrim: {Key: schemas.Krutrim, DefaultBaseURL: "https://cloud.olakrutrim.com", PathPrefix: "v1", Embedding: false},
	schemas.Sensenova: {Key: schemas.Sensenova, DefaultBaseURL: "https://api.sensenova.cn/compatible-mode", PathPrefix: "v1", Embedding: false},
	schemas.Spark: {Key: schemas.Spark, DefaultBaseURL: "https://spark-api-open.xf-yun.com", PathPrefix: "v1", Embedding: false},
	schemas.Reka: {Key: schemas.Reka, DefaultBaseURL: "https://api.reka.ai", PathPrefix: "v1", Embedding: false},
	schemas.Featherless: {Key: schemas.Featherless, DefaultBaseURL: "https://api.featherless.ai", PathPrefix: "v1", Embedding: false},
	schemas.Scaleway: {Key: schemas.Scaleway, DefaultBaseURL: "https://api.scaleway.ai", PathPrefix: "v1", Embedding: false},
	schemas.Voyage: {Key: schemas.Voyage, DefaultBaseURL: "https://api.voyageai.com", PathPrefix: "v1", Embedding: true},
	schemas.Jina: {Key: schemas.Jina, DefaultBaseURL: "https://api.jina.ai", PathPrefix: "v1", Embedding: true},
	schemas.Publicai: {Key: schemas.Publicai, DefaultBaseURL: "https://api.publicai.co", PathPrefix: "v1", Embedding: false},
	schemas.Inferencenet: {Key: schemas.Inferencenet, DefaultBaseURL: "https://api.inference.net", PathPrefix: "v1", Embedding: false},
	schemas.Lingyiwanwu: {Key: schemas.Lingyiwanwu, DefaultBaseURL: "https://api.lingyiwanwu.com", PathPrefix: "v1", Embedding: false},
	schemas.Inceptionlabs: {Key: schemas.Inceptionlabs, DefaultBaseURL: "https://api.inceptionlabs.ai", PathPrefix: "v1", Embedding: false},
	schemas.Nousresearch: {Key: schemas.Nousresearch, DefaultBaseURL: "https://inference-api.nousresearch.com", PathPrefix: "v1", Embedding: false},
	schemas.Morphllm: {Key: schemas.Morphllm, DefaultBaseURL: "https://api.morphllm.com", PathPrefix: "v1", Embedding: false},
	schemas.Nlpcloud: {Key: schemas.Nlpcloud, DefaultBaseURL: "https://api.nlpcloud.io", PathPrefix: "v1", Embedding: false},
	schemas.Aionlabs: {Key: schemas.Aionlabs, DefaultBaseURL: "https://api.aionlabs.ai", PathPrefix: "v1", Embedding: false},
	schemas.Totalgpt: {Key: schemas.Totalgpt, DefaultBaseURL: "https://api.totalgpt.ai", PathPrefix: "v1", Embedding: false},
	schemas.Mancer: {Key: schemas.Mancer, DefaultBaseURL: "https://mancer.tech/oai", PathPrefix: "v1", Embedding: false},
	schemas.Dit: {Key: schemas.Dit, DefaultBaseURL: "https://api.dit.ai", PathPrefix: "v1", Embedding: false},
	schemas.Opper: {Key: schemas.Opper, DefaultBaseURL: "https://api.opper.ai/v3/compat", PathPrefix: "", Embedding: false},
	schemas.Openadapter: {Key: schemas.Openadapter, DefaultBaseURL: "https://api.openadapter.in", PathPrefix: "v1", Embedding: false},
	schemas.Nanogpt: {Key: schemas.Nanogpt, DefaultBaseURL: "https://nano-gpt.com/api", PathPrefix: "v1", Embedding: false},
	schemas.Freemodel: {Key: schemas.Freemodel, DefaultBaseURL: "https://api.freemodel.dev", PathPrefix: "v1", Embedding: false},
	schemas.Freeai: {Key: schemas.Freeai, DefaultBaseURL: "https://api.free.ai", PathPrefix: "v1", Embedding: false},
	schemas.Fastinfra: {Key: schemas.Fastinfra, DefaultBaseURL: "https://api.fastinfra.ai", PathPrefix: "v1", Embedding: false},
	schemas.Wafer: {Key: schemas.Wafer, DefaultBaseURL: "https://pass.wafer.ai", PathPrefix: "v1", Embedding: false},
	schemas.Gmiserving: {Key: schemas.Gmiserving, DefaultBaseURL: "https://api.gmi-serving.com", PathPrefix: "v1", Embedding: false},
	schemas.Cerebrium: {Key: schemas.Cerebrium, DefaultBaseURL: "https://api.cerebrium.ai", PathPrefix: "v1", Embedding: false},
	schemas.Dashscopecn: {Key: schemas.Dashscopecn, DefaultBaseURL: "https://dashscope.aliyuncs.com/compatible-mode", PathPrefix: "v1", Embedding: false},
}

