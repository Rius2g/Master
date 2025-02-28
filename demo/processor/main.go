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

    longReleaseTime := time.Now().Add(30 * time.Second)
    err = c.Upload( 
        "This message will be released in 30 seconds",
        "Demo user",
        "slow-release",
        longReleaseTime.Unix(),
        [][]byte{},
        1,
    )

    if err != nil {
        log.Fatal(err)
    } else {
        fmt.Println("Message uploaded: slow-release")
    }

    time.Sleep(15 * time.Second)
    
    fmt.Println("\n\n==== DEMO WORKFLOW PART 2: Message processing ====")
    
   fmt.Println("Creating a causal chain of messages...")
	firstMessageRelease := time.Now().Add(15 * time.Second)
	err = c.Upload(
		"First message in causal chain",
		"Demo User",
		"causal-1",
		firstMessageRelease.Unix(),
		[][]byte{}, // No dependencies
		1,          // Security level
	)
	if err != nil {
		log.Printf("Failed to upload first causal message: %v", err)
	} else {
		fmt.Printf("✅ Uploaded first message in causal chain (release at: %s)\n", 
			firstMessageRelease.Format(time.RFC3339))
	}
	
	// Wait for blockchain processing and for the message processor to capture the hash
	time.Sleep(5 * time.Second)
	
	// Get the hash from the message processor
	firstHash, found := mp.GetDependencyHash("causal-1")
	if !found {
		fmt.Println("⚠️ Message processor hasn't captured the hash yet. Using placeholder for demo.")
		// Second message without dependency
		secondMessageRelease := time.Now().Add(25 * time.Second)
		err = c.Upload(
			"Second message in causal chain (placeholder dependency)",
			"Demo User",
			"causal-2",
			secondMessageRelease.Unix(),
			[][]byte{},  // Empty dependency list
			1,           // Security level
		)
		if err != nil {
			log.Printf("Failed to upload second causal message: %v", err)
		} else {
			fmt.Printf("Uploaded second message in causal chain (release at: %s)\n", 
				secondMessageRelease.Format(time.RFC3339))
			fmt.Println("   Note: Using placeholder dependency for demonstration")
		}
	} else {
		// Second message with real dependency on first
		fmt.Println("Message processor captured the hash from the first message")
		secondMessageRelease := time.Now().Add(25 * time.Second)
		err = c.Upload(
			"Second message in causal chain (depends on first)",
			"Demo User",
			"causal-2",
			secondMessageRelease.Unix(),
			[][]byte{firstHash}, // Real dependency on first message
			1,                   // Security level
		)
		if err != nil {
			log.Printf("Failed to upload second causal message: %v", err)
		} else {
			fmt.Printf("Uploaded second message with real dependency on first message\n")
			fmt.Printf("   Release time: %s\n", secondMessageRelease.Format(time.RFC3339))
			fmt.Println("   The message processor will enforce proper causal ordering")
		}
	}
	
	// Demo Part 3: Security Levels
	time.Sleep(5 * time.Second)
	fmt.Println("\n=== DEMO PART 3: Security Levels ===")
	
	// Message with standard security level
	err = c.Upload(
		"This message has standard security level (1)",
		"Demo User",
		"security-standard",
		time.Now().Add(20 * time.Second).Unix(),
		[][]byte{},  // No dependencies
		1,           // Standard security level
	)
	if err != nil {
		log.Printf("Failed to upload standard security message: %v", err)
	} else {
		fmt.Println("   The message processor enforces security level constraints")
	}
	
	fmt.Println("\nDemo is running. Watch for messages to appear as they are released...")
	fmt.Println("The full demo will take about 40 seconds to complete.")
	fmt.Println("The message processor is automatically handling the two-phase dissemination,")
	fmt.Println("dependency tracking, and security level enforcement.")
    
}
