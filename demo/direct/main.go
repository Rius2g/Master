package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	
	// Import your backend package
	contract "github.com/rius2g/Master/backend/pkg/ContractInteractionInterface"
	t "github.com/rius2g/Master/backend/pkg/types"
)


//this is the demo to show the contract interface package without the message processor abstraction
func main(){
    
    if err := godotenv.Load(); err != nil {
        log.Fatal("No .env file found") 
    }

    fmt.Println("==== Smart contract direct interaction Demo ====")
    fmt.Println("Initializing contract interaction interface")

    contractAddress := os.Getenv("CONTRACT_ADDRESS")
    if contractAddress == "" {
        log.Fatal("CONTRACT_ADDRESS not set in .env")
    }

    privateKey := os.Getenv("PRIVATE_KEY") 
    if privateKey == "" { 
        log.Fatal("PRIVATE_KEY not set in .env")
    }

    c, err := contract.Init(contractAddress, privateKey, 1) //set security level as 1
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println("Contract interaction interface initialized")

    ctx, cancel := context.WithCancel(context.Background()) 
    defer cancel()

    messages, encryptedMessages, err := c.Listen(ctx)
    if err != nil {
        log.Fatal(err)
    }
    

}
