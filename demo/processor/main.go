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
    msgProcessor "github.com/rius2g/Master/backend/pkg/messageProcessor"
	t "github.com/rius2g/Master/backend/pkg/types"
)


//this is the demo to show the package with the message processor abstraction
func main(){
    if err := godotenv.Load(); err != nil {
        log.Fatal("No .env file found")
    }

    fmt.Println("==== Smart Contract Message Processor Demo ====")
    fmt.Println("Initializing Smart Contract interface...") 

    contractAddress := os.Getenv("CONTRACT_ADDRESS") 
    if contractAddress == "" {
        log.Fatal("CONTRACT_ADDRESS not found in .env")
    }

    privateKey := os.Getenv("PRIVATE_KEY") 
    if privateKey == "" {
        log.Fatal("PRIVATE_KEY not found in .env")
    }

    c, err := contract.Init(contractAddress, privateKey, 1) //security level 1
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println("Smart Contract interface initialized") 

    mp, err := msgProcessor.NewMessageProcessor(c)
    if err != nil { 
        log.Fatal(err)
    }

    fmt.Println("Message Processor initialized")

    mp.Start() 
    defer mp.Stop() 

    go func(){
        for err := range mp.Errors(){
            log.Println(err)
        }
    }()

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    messages, encryptedMessages, err := c.Listen(ctx)
    if err != nil {
        log.Fatal(err)
    }
    
    go monitorMessages(messages, encryptedMessages, ctx)

    go runDemoWorkflow(c, mp)
    
    fmt.Println("Demo is running. Press Ctrl+C to stop") 
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM) 
    <-sigChan
}

func monitorMessages(messages <-chan t.Message, encryptedMessages <-chan t.EncryptedMessage, ctx context.Context) {
	for {
		select {
		case msg := <-messages:
			fmt.Printf("\n [MONITOR] DECRYPTED MESSAGE: %s\n", msg.DataName)
			fmt.Printf("  Content: %s\n", msg.Content)
			fmt.Println("  (Being processed by message processor...)")
		case encMsg := <-encryptedMessages:
			fmt.Printf("\n [MONITOR] ENCRYPTED MESSAGE: %s\n", encMsg.DataName)
			fmt.Printf("  Release Time: %s\n", time.Unix(encMsg.ReleaseTime.Int64(), 0).Format(time.RFC3339))
			fmt.Println("  (Being processed by message processor...)")
		case <-ctx.Done():
			return
		}
	}
}



func runDemoWorkflow(c *contract.ContractInteractionInterface, mp *msgProcessor.MessageProcessor){
   time.Sleep(2 * time.Second)

   fmt.Println("\n\n==== DEMO WORKFLOW PART 1: Time-sensitive dissemination ====") 

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
}
