package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"os"

	"golang.org/x/crypto/scrypt"
)

// Beware: This was generated in a haste using an LLM, do not trust it

// EncryptedStore stores an encryption password used to derive keys for encryption and decryption.
type EncryptedStore struct {
	Password string
}

const (
	saltSize = 16            // Size in bytes for the salt.
	ivSize   = aes.BlockSize // AES block size is 16 bytes.
)

// deriveKey derives a 32-byte key from the given password and salt using scrypt.
// These parameters (N=32768, r=8, p=1) provide a stronger derivation than a simple hash.
func deriveKey(password string, salt []byte) ([]byte, error) {
	return scrypt.Key([]byte(password), salt, 32768, 8, 1, 32)
}

// OpenReader opens an encrypted file for reading. The file is expected to have a header:
// [salt (16 bytes)] [IV (16 bytes)] followed by the encrypted content.
// It returns an io.ReadCloser that decrypts data on the fly.
func (es *EncryptedStore) OpenReader(path string) (io.ReadCloser, error) {
	// Open the file for reading.
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file for reading: %w", err)
	}

	// Read the salt.
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(f, salt); err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to read salt: %w", err)
	}

	// Read the IV.
	iv := make([]byte, ivSize)
	if _, err := io.ReadFull(f, iv); err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to read IV: %w", err)
	}

	// Derive the encryption key using scrypt.
	key, err := deriveKey(es.Password, salt)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to derive key: %w", err)
	}

	// Create the AES cipher.
	block, err := aes.NewCipher(key)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Create a stream cipher (CTR mode) for decryption.
	stream := cipher.NewCTR(block, iv)
	streamReader := &cipher.StreamReader{
		S: stream,
		R: f,
	}

	// Return a ReadCloser that uses the stream reader and the underlying file.
	return struct {
		io.Reader
		io.Closer
	}{
		Reader: streamReader,
		Closer: f,
	}, nil
}

// OpenWriter opens (or creates) a file for writing encrypted data.
// It writes a header containing a randomly generated salt and IV, then returns an io.WriteCloser
// that encrypts data on the fly. If the file does not exist, it is created.
func (es *EncryptedStore) OpenWriter(path string) (io.WriteCloser, error) {
	// Open (or create) the file with write permissions.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open file for writing: %w", err)
	}

	// Generate a random salt.
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	// Generate a random IV.
	iv := make([]byte, ivSize)
	if _, err := rand.Read(iv); err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to generate IV: %w", err)
	}

	// Write the salt and IV to the file.
	if _, err := f.Write(salt); err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to write salt: %w", err)
	}
	if _, err := f.Write(iv); err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to write IV: %w", err)
	}

	// Derive the encryption key using scrypt.
	key, err := deriveKey(es.Password, salt)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to derive key: %w", err)
	}

	// Create the AES cipher.
	block, err := aes.NewCipher(key)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Create a stream cipher (CTR mode) for encryption.
	stream := cipher.NewCTR(block, iv)
	streamWriter := &cipher.StreamWriter{
		S: stream,
		W: f,
	}

	// Return a WriteCloser that encrypts data and closes the underlying file.
	return struct {
		io.Writer
		io.Closer
	}{
		Writer: streamWriter,
		Closer: f,
	}, nil
}
