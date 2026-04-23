package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const encPrefix = "enc:v1:"

func main() {
	if len(os.Args) < 2 {
		fail("usage: weknora_admin_helper.go generate --tenant-id <id> --password <plaintext>")
	}

	switch os.Args[1] {
	case "generate":
		if err := runGenerate(os.Args[2:]); err != nil {
			fail(err.Error())
		}
	default:
		fail("unknown command: " + os.Args[1])
	}
}

func runGenerate(args []string) error {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	tenantID := fs.Uint64("tenant-id", 0, "tenant id used to generate the tenant API key")
	password := fs.String("password", "", "plaintext password to hash with bcrypt")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *tenantID == 0 {
		return errors.New("--tenant-id must be greater than 0")
	}
	if *password == "" {
		return errors.New("--password is required")
	}

	tenantAESKey := []byte(os.Getenv("TENANT_AES_KEY"))
	if len(tenantAESKey) != 32 {
		return errors.New("TENANT_AES_KEY must be exactly 32 bytes")
	}
	systemAESKey := []byte(os.Getenv("SYSTEM_AES_KEY"))
	if len(systemAESKey) != 32 {
		return errors.New("SYSTEM_AES_KEY must be exactly 32 bytes")
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("generate bcrypt hash: %w", err)
	}

	tenantAPIKey, err := generateTenantAPIKey(*tenantID, tenantAESKey)
	if err != nil {
		return fmt.Errorf("generate tenant api key: %w", err)
	}

	encryptedTenantAPIKey, err := encryptAESGCM(tenantAPIKey, systemAESKey)
	if err != nil {
		return fmt.Errorf("encrypt tenant api key: %w", err)
	}

	// Output uses simple KEY=VALUE lines so the shell wrapper can parse values
	// without eval. Bcrypt hashes contain '$', so callers must read these lines
	// rather than source them.
	fmt.Printf("PASSWORD_HASH=%s\n", string(passwordHash))
	fmt.Printf("TENANT_API_KEY=%s\n", tenantAPIKey)
	fmt.Printf("ENCRYPTED_TENANT_API_KEY=%s\n", encryptedTenantAPIKey)
	return nil
}

func generateTenantAPIKey(tenantID uint64, secret []byte) (string, error) {
	idBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(idBytes, tenantID)

	block, err := aes.NewCipher(secret)
	if err != nil {
		return "", err
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := aesgcm.Seal(nil, nonce, idBytes, nil)
	combined := append(nonce, ciphertext...)
	encoded := base64.RawURLEncoding.EncodeToString(combined)
	return "sk-" + encoded, nil
}

func encryptAESGCM(plaintext string, key []byte) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if strings.HasPrefix(plaintext, encPrefix) {
		return plaintext, nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, aesgcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := aesgcm.Seal(nil, nonce, []byte(plaintext), nil)
	combined := append(nonce, ciphertext...)
	return encPrefix + base64.RawURLEncoding.EncodeToString(combined), nil
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}