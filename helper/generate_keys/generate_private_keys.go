package main

import (
	"crypto/ecdsa"
	"fmt"
	"log"
	"os"

	"github.com/ethereum/go-ethereum/crypto"
)

func main() {
	const walletCount = 20

	err := os.MkdirAll("env_keys", os.ModePerm)
	if err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	for i := 1; i <= walletCount; i++ {
		// Generate private key
		privateKey, err := crypto.GenerateKey()
		if err != nil {
			log.Fatalf("Failed to generate key %d: %v", i, err)
		}

		// Derive hex-encoded private key
		privateKeyBytes := crypto.FromECDSA(privateKey)
		privateKeyHex := fmt.Sprintf("0x%x", privateKeyBytes)

		// Derive public address
		publicKey := privateKey.Public().(*ecdsa.PublicKey)
		address := crypto.PubkeyToAddress(*publicKey)

		// Save to .env.key{i} file
		filename := fmt.Sprintf("env_keys/.env.key%d", i)
		fileContent := fmt.Sprintf("PRIVATE_KEY=%s\nCONTRACT_ADDRESS=\n", privateKeyHex)
		if err := os.WriteFile(filename, []byte(fileContent), 0600); err != nil {
			log.Fatalf("Failed to write key file %d: %v", i, err)
		}

		// Print to stdout for funding
		fmt.Printf("Key %2d → Address: %s\n", i, address.Hex())
	}
}
