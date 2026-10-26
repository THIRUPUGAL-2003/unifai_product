package tables

import (
	"testing"

	"github.com/gateway/gateway/core/schemas"
	"github.com/gateway/gateway/framework/encrypt"
)

const (
	testOldKey = "old-encryption-passphrase-0001"
	testNewKey = "new-encryption-passphrase-0002"
)

// withKey switches the process-wide encryption key for one step of a test.
func withKey(t *testing.T, key string) {
	t.Helper()
	encrypt.Init(key, nil)
	t.Cleanup(func() { encrypt.Init("", nil) })
}

func TestSecretVarUndecryptableIsKeptAndWrittenBackUnchanged(t *testing.T) {
	withKey(t, testOldKey)
	stored := schemas.NewSecretVar("sk-live-secret")
	if err := encryptSecretVar(stored); err != nil {
		t.Fatal(err)
	}
	ciphertext := stored.Val

	// Key changed: the row loads, but the value must not be blanked.
	withKey(t, testNewKey)
	loaded := schemas.NewSecretVar(ciphertext)
	if err := decryptSecretVar(loaded); err != nil {
		t.Fatalf("decrypt failure must not stop loading: %v", err)
	}
	if loaded.Val != ciphertext {
		t.Fatalf("undecryptable value was changed to %q", loaded.Val)
	}
	// Saving writes the original ciphertext back, not a double-encrypted value.
	if err := encryptSecretVar(loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.Val != ciphertext {
		t.Fatal("undecryptable value was re-encrypted on save")
	}

	// With the old key back, the original secret is still recoverable.
	withKey(t, testOldKey)
	if err := decryptSecretVar(loaded); err != nil || loaded.Val != "sk-live-secret" {
		t.Fatalf("secret not recoverable with the old key: %q, %v", loaded.Val, err)
	}
}

func TestStringUndecryptableIsKept(t *testing.T) {
	withKey(t, testOldKey)
	value := "smtp-password"
	if err := encryptString(&value); err != nil {
		t.Fatal(err)
	}
	ciphertext := value

	withKey(t, testNewKey)
	if err := decryptString(&value); err != nil || value != ciphertext {
		t.Fatalf("decryptString blanked or failed: %q, %v", value, err)
	}
	if err := encryptString(&value); err != nil || value != ciphertext {
		t.Fatalf("encryptString re-encrypted kept ciphertext: %v", err)
	}
}

func TestNormalValuesStillRoundTrip(t *testing.T) {
	withKey(t, testNewKey)
	secret := schemas.NewSecretVar("sk-new")
	if err := encryptSecretVar(secret); err != nil || secret.Val == "sk-new" {
		t.Fatalf("value was not encrypted: %v", err)
	}
	if err := decryptSecretVar(secret); err != nil || secret.Val != "sk-new" {
		t.Fatalf("round trip failed: %q, %v", secret.Val, err)
	}
	s := "plain"
	if err := encryptString(&s); err != nil || s == "plain" {
		t.Fatal("string was not encrypted")
	}
	if err := decryptString(&s); err != nil || s != "plain" {
		t.Fatalf("string round trip failed: %q", s)
	}
}

func TestPluginConfigUndecryptableIsRestoredOnSave(t *testing.T) {
	withKey(t, testOldKey)
	orig := &TablePlugin{Name: "test-plugin-restore", Config: map[string]any{"token": "abc"}}
	if err := orig.BeforeSave(nil); err != nil {
		t.Fatal(err)
	}
	ciphertext := orig.ConfigJSON

	withKey(t, testNewKey)
	loaded := &TablePlugin{Name: orig.Name, ConfigJSON: ciphertext, EncryptionStatus: EncryptionStatusEncrypted}
	if err := loaded.AfterFind(nil); err != nil {
		t.Fatalf("server must still load the plugin: %v", err)
	}
	if err := loaded.BeforeSave(nil); err != nil {
		t.Fatal(err)
	}
	if loaded.ConfigJSON != ciphertext {
		t.Fatal("re-saving the loaded row wiped the stored config")
	}
	// A struct rebuilt from in-memory config (no Config) must not wipe it either.
	fresh := &TablePlugin{Name: orig.Name}
	if err := fresh.BeforeSave(nil); err != nil {
		t.Fatal(err)
	}
	if fresh.ConfigJSON != ciphertext || fresh.EncryptionStatus != EncryptionStatusEncrypted {
		t.Fatal("fresh save wiped the stored config")
	}
	// A deliberately new config replaces it, and later empty saves stay empty.
	updated := &TablePlugin{Name: orig.Name, Config: map[string]any{"token": "new"}}
	if err := updated.BeforeSave(nil); err != nil {
		t.Fatal(err)
	}
	if updated.ConfigJSON == ciphertext {
		t.Fatal("new config did not replace the old ciphertext")
	}
	cleared := &TablePlugin{Name: orig.Name}
	if err := cleared.BeforeSave(nil); err != nil {
		t.Fatal(err)
	}
	if cleared.ConfigJSON != "{}" {
		t.Fatalf("old ciphertext came back after a new config was saved: %q", cleared.ConfigJSON)
	}
}

func TestMCPHeadersAndConnectionStringUndecryptableAreKept(t *testing.T) {
	withKey(t, testOldKey)
	orig := &TableMCPClient{
		ClientID:         "test-mcp-restore",
		Name:             "mcp",
		ConnectionString: schemas.NewSecretVar("https://mcp.example/sse?token=x"),
		Headers:          map[string]schemas.SecretVar{"Authorization": *schemas.NewSecretVar("Bearer t")},
	}
	if err := orig.BeforeSave(nil); err != nil {
		t.Fatal(err)
	}
	headers, conn := orig.HeadersJSON, orig.ConnectionString.Val

	withKey(t, testNewKey)
	loaded := &TableMCPClient{
		ClientID:         orig.ClientID,
		HeadersJSON:      headers,
		ConnectionString: schemas.NewSecretVar(conn),
		EncryptionStatus: EncryptionStatusEncrypted,
	}
	if err := loaded.AfterFind(nil); err != nil {
		t.Fatalf("server must still load the MCP client: %v", err)
	}
	if err := loaded.BeforeSave(nil); err != nil {
		t.Fatal(err)
	}
	if loaded.HeadersJSON != headers {
		t.Fatal("re-saving wiped the stored MCP headers")
	}
	if loaded.ConnectionString.Val != conn {
		t.Fatal("re-saving changed the stored MCP connection string")
	}
}

func TestProviderProxySMTPAndAlertChannelUndecryptableAreKept(t *testing.T) {
	withKey(t, testOldKey)
	prov := &TableProvider{Name: "test-provider-restore", ProxyConfig: &schemas.ProxyConfig{Type: "http", URL: schemas.NewSecretVar("http://proxy:8080")}}
	if err := prov.BeforeSave(nil); err != nil {
		t.Fatal(err)
	}
	smtp := &TableSMTPConfig{ID: 7, Password: "mail-pass"}
	if err := smtp.BeforeSave(nil); err != nil {
		t.Fatal(err)
	}
	alert := &TableAlertChannel{ID: 9, ParsedConfig: map[string]any{"webhook": "https://hooks.example/x"}}
	if err := alert.BeforeSave(nil); err != nil {
		t.Fatal(err)
	}
	proxyCipher, smtpCipher, alertCipher := prov.ProxyConfigJSON, smtp.Password, *alert.ConfigJSON

	withKey(t, testNewKey)
	p2 := &TableProvider{Name: prov.Name, ProxyConfigJSON: proxyCipher, EncryptionStatus: EncryptionStatusEncrypted}
	if err := p2.AfterFind(nil); err != nil {
		t.Fatal(err)
	}
	p2.ProxyConfig = nil
	if err := p2.BeforeSave(nil); err != nil {
		t.Fatal(err)
	}
	if p2.ProxyConfigJSON != proxyCipher {
		t.Fatal("provider proxy config was wiped")
	}

	s2 := &TableSMTPConfig{ID: 7, Password: smtpCipher, EncryptionStatus: EncryptionStatusEncrypted}
	if err := s2.AfterFind(nil); err != nil {
		t.Fatal(err)
	}
	if err := s2.BeforeSave(nil); err != nil {
		t.Fatal(err)
	}
	if s2.Password != smtpCipher {
		t.Fatal("SMTP password was wiped or re-encrypted")
	}
	freshSMTP := &TableSMTPConfig{ID: 7}
	if err := freshSMTP.BeforeSave(nil); err != nil {
		t.Fatal(err)
	}
	if freshSMTP.Password != smtpCipher {
		t.Fatal("empty SMTP save wiped the stored password")
	}

	a2 := &TableAlertChannel{ID: 9, ConfigJSON: &alertCipher, EncryptionStatus: EncryptionStatusEncrypted}
	if err := a2.AfterFind(nil); err != nil {
		t.Fatal(err)
	}
	if err := a2.BeforeSave(nil); err != nil {
		t.Fatal(err)
	}
	if a2.ConfigJSON == nil || *a2.ConfigJSON != alertCipher {
		t.Fatal("alert channel config was wiped")
	}
}
