package tables

import (
	"errors"
	"sync"

	"github.com/gateway/gateway/core/schemas"
	"github.com/gateway/gateway/framework/encrypt"
)

const (
	// EncryptionStatusPlainText indicates the row's sensitive fields are stored as plaintext.
	EncryptionStatusPlainText = "plain_text"
	// EncryptionStatusEncrypted indicates the row's sensitive fields have been encrypted.
	EncryptionStatusEncrypted = "encrypted"
)

// undecryptable holds stored ciphertexts that the current encryption key cannot open
// (for example after GATEWAY_ENCRYPTION_KEY was changed). Such values are kept as-is
// in memory instead of being blanked, and the save path writes them back unchanged,
// so a key change never silently wipes stored secrets. The server still starts.
var undecryptable sync.Map

// decryptOrKeep returns the plaintext of a stored ciphertext. When it cannot be
// decrypted, it returns the ciphertext unchanged and remembers it (see undecryptable).
func decryptOrKeep(value string) string {
	if value == "" {
		return value
	}
	decrypted, err := encrypt.Decrypt(value)
	if err == nil {
		return decrypted
	}
	if _, seen := undecryptable.LoadOrStore(value, struct{}{}); !seen && !errors.Is(err, encrypt.ErrEncryptionKeyNotInitialized) {
		encrypt.Warn("configstore: a stored secret cannot be decrypted with the current encryption key; keeping the stored value unchanged (re-enter it or restore the previous key): %v", err)
	}
	return value
}

// encryptOrKeep encrypts a value for storage, except a value that was loaded as
// undecryptable ciphertext: that one is written back unchanged, never re-encrypted.
func encryptOrKeep(value string) (string, error) {
	if value == "" || isUndecryptable(value) {
		return value, nil
	}
	return encrypt.Encrypt(value)
}

// isUndecryptable reports whether value is stored ciphertext the current key cannot open.
func isUndecryptable(value string) bool {
	_, ok := undecryptable.Load(value)
	return ok
}

// keptRowCiphertext maps "table/row/column" to a stored encrypted JSON blob that the
// current key could not decrypt. Such blobs are parsed as empty so the server starts,
// but are re-serialized on save; restoreKeptCiphertext puts the original back when the
// row would otherwise be saved empty, including from a freshly built struct.
var keptRowCiphertext sync.Map

func rowCiphertextKey(table, row, column string) string {
	return table + "/" + row + "/" + column
}

// decryptRowBlob decrypts a stored encrypted blob. On failure it remembers the stored
// ciphertext for this row/column and returns ok=false; the caller uses an empty value.
func decryptRowBlob(table, row, column, stored string) (plaintext string, ok bool) {
	decrypted, err := encrypt.Decrypt(stored)
	if err == nil {
		return decrypted, true
	}
	if row != "" {
		if _, seen := keptRowCiphertext.Swap(rowCiphertextKey(table, row, column), stored); !seen && !errors.Is(err, encrypt.ErrEncryptionKeyNotInitialized) {
			encrypt.Warn("configstore: %s %q %s cannot be decrypted with the current encryption key; keeping the stored value unchanged (re-enter it or restore the previous key): %v", table, row, column, err)
		}
	}
	return "", false
}

// restoreKeptCiphertext is called on save with the freshly serialized value. If the
// value is empty and the row still has undecryptable stored ciphertext, it returns that
// ciphertext (restored=true) so it is written back unchanged and not re-encrypted.
// A non-empty value is a deliberate new setting and replaces the old ciphertext.
func restoreKeptCiphertext(table, row, column, value string, isEmpty bool) (string, bool) {
	if row == "" {
		return value, false
	}
	key := rowCiphertextKey(table, row, column)
	kept, ok := keptRowCiphertext.Load(key)
	if !ok {
		return value, false
	}
	if isEmpty || value == kept.(string) {
		// Empty, or the same undecryptable ciphertext written back: keep it as stored.
		return kept.(string), true
	}
	keptRowCiphertext.Delete(key)
	return value, false
}

// encryptSecretVar encrypts the Val field of an SecretVar in place using AES-256-GCM.
// It is a no-op if the field is nil, references an environment variable or vault, or has an empty value.
func encryptSecretVar(field *schemas.SecretVar) error {
	if field == nil || field.IsFromSecret() || field.GetValue() == "" {
		return nil
	}
	encrypted, err := encryptOrKeep(field.Val)
	if err != nil {
		return err
	}
	field.Val = encrypted
	return nil
}

// decryptSecretVar decrypts the Val field of an SecretVar in place using AES-256-GCM.
// It is a no-op if the field is nil, references an environment variable or vault, or has an empty value.
// A value the current key cannot decrypt is kept as stored (see decryptOrKeep).
func decryptSecretVar(field *schemas.SecretVar) error {
	if field == nil || field.IsFromSecret() || field.GetValue() == "" {
		return nil
	}
	field.Val = decryptOrKeep(field.Val)
	return nil
}

// encryptSecretVarPtr encrypts the Val field of a pointer-to-SecretVar in place.
// It is a no-op if the pointer or the SecretVar it points to is nil.
func encryptSecretVarPtr(field **schemas.SecretVar) error {
	if field == nil || *field == nil {
		return nil
	}
	return encryptSecretVar(*field)
}

// decryptSecretVarPtr decrypts the Val field of a pointer-to-SecretVar in place.
// It is a no-op if the pointer or the SecretVar it points to is nil.
func decryptSecretVarPtr(field **schemas.SecretVar) error {
	if field == nil || *field == nil {
		return nil
	}
	return decryptSecretVar(*field)
}

// encryptString encrypts the string pointed to by value in place using AES-256-GCM.
// It is a no-op if the pointer is nil or the string is empty.
func encryptString(value *string) error {
	if value == nil || *value == "" {
		return nil
	}
	encrypted, err := encryptOrKeep(*value)
	if err != nil {
		return err
	}
	*value = encrypted
	return nil
}

// decryptString decrypts the string pointed to by value in place using AES-256-GCM.
// It is a no-op if the pointer is nil or the string is empty.
// A value the current key cannot decrypt is kept as stored (see decryptOrKeep).
func decryptString(value *string) error {
	if value == nil || *value == "" {
		return nil
	}
	*value = decryptOrKeep(*value)
	return nil
}
