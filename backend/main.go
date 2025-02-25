package main


import (
    "fmt"
    "log"
    "os"
    "os/signal"
    "syscall"

   "github.com/joho/godotenv"
    contract "github.com/rius2g/Master/backend/pkg/ContractInteractionInterface"
    msgProcessor "github.com/rius2g/Master/backend/pkg/messageProcessor"
)


//this is the package usage example
func main() {
    if err := godotenv.Load(); err != nil {
        log.Fatal("Error loading .env file") 
    }
    fmt.Println("Starting the application...")


    fmt.Println("ABI loaded successfully")

    contractAddress := os.Getenv("CONTRACT_ADDRESS") 
    privateKey := os.Getenv("PRIVATE_KEY")

    securityLevel := uint(1)
    
    c, err := contract.Init(contractAddress, privateKey, securityLevel)
    if err != nil {
        log.Fatalf("failed to initialize contract: %v", err)
    }

    fmt.Println("Contract initialized successfully")

    mp, err := msgProcessor.NewMessageProcessor(c)
    if err != nil {
        log.Fatalf("failed to create message processor: %v", err)
    }

    fmt.Println("Message Processor initialized successfully")
    
    //listen to events from the smart contract, has to be run at all times to receive both encrypted data and the encrypted datas private keys
    mp.Start()

    defer mp.Stop()

    go func(){
        for err := range mp.Errors() {
            log.Printf("error processing message: %v", err)
        }
    }()

    sigChan := make (chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
    <-sigChan

    //lets make a demo program?

}


