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


    go handleMessages(messages, encryptedMessages, ctx)

    go runDemoWorkflow(c)

    fmt.Println("Demo is running. Press Ctrl+C to stop") 
    sigChan := make(chan os.Signal, 1) 
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    <-sigChan 

    fmt.Println("Shutting down...")
}


func handleMessages(messages <-chan t.Message, encryptedMessages <-chan t.EncryptedMessage, ctx context.Context) {
    for {
        select {
        case msg := <-messages:
            printDecryptedMessage(msg)
        case encryptedMsg := <-encryptedMessages:
            printEncryptedMessages(encryptedMsg) 
        case <-ctx.Done():
            return
        }
    }
}



func printDecryptedMessage(msg t.Message){
    fmt.Printf("Decrypted message: %s\n", msg.DataName) 
    fmt.Printf("  From: %s\n", msg.Owner)
    fmt.Printf("  To: %s\n", msg.Content)
    fmt.Printf("  Received at: %s\n", time.Now().Format(time.RFC3339))
    
    if len(msg.Dependencies) > 0 {
        fmt.Printf("   Dependencies: %d\n", len(msg.Dependencies))
    }

    if len(msg.VectorClocks) > 0 {
        fmt.Printf("   Vector clocks: %d\n", len(msg.VectorClocks))
        for _, vc := range msg.VectorClocks {
            fmt.Printf("   - Process %s: %s\n", vc.Process.String(), vc.TimeStamp.String())
        }
    }

    fmt.Println()
}


func printEncryptedMessages(msg t.EncryptedMessage){
    fmt.Printf("\n ENCRYPTED MESSAGE: %s\n", msg.DataName) 
    fmt.Printf("  From: %s\n", msg.Owner)
    fmt.Printf("  Received at: %s\n", time.Now().Format(time.RFC3339))
    
    if len(msg.Dependencies) > 0 {
        fmt.Printf("   Dependencies: %d\n", len(msg.Dependencies))
    }

    if len(msg.VectorClocks) > 0 {
        fmt.Printf("   Vector clocks: %d\n", len(msg.VectorClocks))
        for _, vc := range msg.VectorClocks {
            fmt.Printf("   - Process %s: %s\n", vc.Process.String(), vc.TimeStamp.String())
        }
    }

    fmt.Println()
}


func runDemoWorkflow(c *contract.ContractInteractionInterface){
    time.Sleep(2 * time.Second) 

    fmt.Println("\n ===== DEMO PART 1: Time-Sensitive Messages =====")

    shortReleaseTime := time.Now().Add(10 * time.Second)
    err := c.Upload(
        "This message will be released in 10 seconds",
        "Demo user",
        "quick-release",
        shortReleaseTime.Unix(),
        [][]byte{},
        1,
    )
    if err != nil {
        log.Fatal(err)
    } else {
        fmt.Println("Message uploaded: quick-release")
    }


    longReleaseTime := time.Now().Add(30 * time.Second) 
    err = c.Upload(
        "This message will be released in 30 seconds",
        "Demo user",
        "Delayed-release",
        longReleaseTime.Unix(),
        [][]byte{},
        1,
    )

    if err != nil {
        log.Fatal(err)
    } else {
        fmt.Println("Message uploaded: Delayed-release")
    }

    time.Sleep(35 * time.Second)

    fmt.Println("\n ===== DEMO PART 2: Casual time ordering & information flow control =====")
    
    messageHashes := make(map[string][]byte)

    fmt.Println("Uploading messages...") 
    firstMessageRelease := time.Now().Add(15 * time.Second)
    err = c.Upload(
        "This is the first message",
        "Demo user",
        "first-message",
        firstMessageRelease.Unix(),
        [][]byte{},
        1,
    )
    if err != nil {
        log.Fatal(err)
    } else {
        fmt.Println("Message uploaded: first-message")
    }

    time.Sleep(5 * time.Second)

    messageHashes["first-message"] = []byte{}
    fmt.Println("In real implementation you need to capture the hash from the encrypted message event")
    fmt.Println("For the demo we are using mock dependencies")

    secondMessageRelease := time.Now().Add(25 * time.Second)
    err = c.Upload(
        "Second message in casual chain (depends on first message)", 
        "Demo user",
        "second-message",
        secondMessageRelease.Unix(),
        [][]byte{},
        1,
    )

    if err != nil {
        log.Fatal(err)
    } else {
        fmt.Println("Message uploaded: second-message")
    }

    time.Sleep(5 * time.Second) 

    fmt.Println("\n ===== DEMO PART 3: Security levels=====")

    err = c.Upload(
        "This message is top secret",
        "Demo user",
        "top secret",
        time.Now().Add(20 * time.Second).Unix(),
        [][]byte{},
        1,
    )

    if err != nil {
        log.Fatal(err)
    } else {
        fmt.Println("Message uploaded: top secret")
    }
    
}
