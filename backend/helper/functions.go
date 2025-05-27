package helper

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"

	"fmt"
	t "github.com/rius2g/Master/backend/pkg/types"
	"log"

	"encoding/json"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

func LoadABI() (abi.ABI, error) {
	filePath := "LamportClock.json"
	abiBytes, err := os.ReadFile(filePath)
	if err != nil {
		return abi.ABI{}, fmt.Errorf("failed to read ABI file: %v", err)
	}

	var contract t.Contract
	err = json.Unmarshal(abiBytes, &contract)
	if err != nil {
		return abi.ABI{}, fmt.Errorf("failed to unmarshal contract JSON: %v", err)
	}

	parsedABI, err := abi.JSON(strings.NewReader(string(contract.ABI)))
	if err != nil {
		return abi.ABI{}, fmt.Errorf("failed to parse ABI JSON: %v", err)
	}

	return parsedABI, nil
}

func EncryptData(data string) ([]byte, []byte, []byte, error) {
	log.Printf("Starting encryption of data with length: %d", len(data))

	easKey := make([]byte, 32) // 256-bit key for AES
	_, err := rand.Read(easKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to generate EAS key: %v", err)
	}
	log.Printf("Generated EAS key of 256 bits")

	aesBlock, err := aes.NewCipher(easKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create AES cipher: %v", err)
	}
	aesGCM, err := cipher.NewGCM(aesBlock)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create GCM block cipher: %v", err)
	}

	nonce := make([]byte, aesGCM.NonceSize())
	_, err = rand.Read(nonce)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to generate nonce: %v", err)
	}

	encryptedData := aesGCM.Seal(nil, nonce, []byte(data), nil)
	log.Printf("Data encrypted using AES-GCM")

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to generate RSA key pair: %v", err)
	}
	log.Printf("Generated RSA key pair")

	encryptedEASKey, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, &privateKey.PublicKey, easKey, nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to encrypt EAS key: %v", err)
	}
	log.Printf("EAS key encrypted using RSA public key")

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	log.Printf("Private key marshaled to bytes")

	return append(nonce, encryptedData...), encryptedEASKey, privateKeyBytes, nil
}

func DecryptData(encryptedData []byte, encryptedEASKey []byte, privateKeyBytes []byte) (string, error) {
	log.Printf("Starting decryption")

	// Parse the private key
	privateKey, err := x509.ParsePKCS1PrivateKey(privateKeyBytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %v", err)
	}
	log.Printf("Private key parsed successfully")

	// Decrypt the EAS key
	easKey, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, privateKey, encryptedEASKey, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt EAS key: %v", err)
	}
	log.Printf("EAS key decrypted successfully")

	// Separate nonce and encrypted data
	nonceSize := 12
	nonce := encryptedData[:nonceSize]
	ciphertext := encryptedData[nonceSize:]

	// Decrypt the data using AES-GCM
	aesBlock, err := aes.NewCipher(easKey)
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %v", err)
	}
	aesGCM, err := cipher.NewGCM(aesBlock)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM block cipher: %v", err)
	}

	decryptedData, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt data: %v", err)
	}
	log.Printf("Data decrypted successfully")

	return string(decryptedData), nil
}

func ValidateHash(decryptedData string, originalHash []byte) bool {
	hash := sha256.Sum256([]byte(decryptedData))
	return bytes.Equal(hash[:], originalHash)
}
