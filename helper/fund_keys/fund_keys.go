package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

const (
	rpcURL        = "https://api.avax-test.network/ext/bc/C/rpc" // Fuji RPC
	mainKeyEnvVar = "MAIN_PRIVATE_KEY"
	amountAVAX    = 0.05
	gasLimit      = uint64(21000)
	gasPriceGwei  = 25 // Fuji usually works with low gas, 25 Gwei is safe
)

func main() {
	mainPrivKeyHex := os.Getenv(mainKeyEnvVar)
	if mainPrivKeyHex == "" {
		log.Fatal("MAIN_PRIVATE_KEY not set in environment")
	}
	if strings.HasPrefix(mainPrivKeyHex, "0x") {
		mainPrivKeyHex = mainPrivKeyHex[2:]
	}
	mainPrivKeyBytes, err := hex.DecodeString(mainPrivKeyHex)
	if err != nil {
		log.Fatalf("Failed to decode private key: %v", err)
	}
	mainPrivKey, err := crypto.ToECDSA(mainPrivKeyBytes)
	if err != nil {
		log.Fatalf("Invalid private key: %v", err)
	}

	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		log.Fatalf("Failed to connect to Avalanche RPC: %v", err)
	}
	defer client.Close()

	fromAddress := crypto.PubkeyToAddress(mainPrivKey.PublicKey)
	nonce, err := client.PendingNonceAt(context.Background(), fromAddress)
	if err != nil {
		log.Fatalf("Failed to get nonce: %v", err)
	}

	// Calculate gas price
	gasPrice := big.NewInt(int64(gasPriceGwei * 1e9))

	// Amount in wei
	amount := new(big.Int).Mul(big.NewInt(int64(amountAVAX*1e18)), big.NewInt(1))

	for i := 1; i <= 20; i++ {
		envFile := fmt.Sprintf("env_keys/.env.key%d", i)
		data, err := os.ReadFile(envFile)
		if err != nil {
			log.Printf("Skipping key %d (read error): %v", i, err)
			continue
		}

		lines := strings.Split(string(data), "\n")
		var privHex string
		for _, line := range lines {
			if strings.HasPrefix(line, "PRIVATE_KEY=") {
				privHex = strings.TrimPrefix(line, "PRIVATE_KEY=")
				privHex = strings.TrimSpace(privHex)
				break
			}
		}
		if privHex == "" {
			log.Printf("No PRIVATE_KEY found in %s", envFile)
			continue
		}
		if strings.HasPrefix(privHex, "0x") {
			privHex = privHex[2:]
		}
		privBytes, err := hex.DecodeString(privHex)
		if err != nil {
			log.Printf("Invalid key in %s: %v", envFile, err)
			continue
		}
		priv, _ := crypto.ToECDSA(privBytes)
		toAddr := crypto.PubkeyToAddress(priv.PublicKey)

		tx := types.NewTransaction(nonce, toAddr, amount, gasLimit, gasPrice, nil)

		signer := types.LatestSignerForChainID(big.NewInt(43113)) // 43113 = Avalanche fuji-Chain
		signedTx, err := types.SignTx(tx, signer, mainPrivKey)
		if err != nil {
			log.Fatalf("Failed to sign tx: %v", err)
		}

		err = client.SendTransaction(context.Background(), signedTx)
		if err != nil {
			log.Printf("Failed to send tx to %s: %v", toAddr.Hex(), err)
			continue
		}

		fmt.Printf("Sent %.2f AVAX to %s | Tx: %s\n", amountAVAX, toAddr.Hex(), signedTx.Hash().Hex())
		nonce++
		time.Sleep(500 * time.Millisecond) // avoid flooding
	}
}
