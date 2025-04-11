package messageProcessor

import (
    "context"
    "fmt"
    "log"
    "github.com/ethereum/go-ethereum/crypto"
    c "github.com/rius2g/Master/backend/pkg/ContractInteractionInterface"
    t "github.com/rius2g/Master/backend/pkg/types"
)

type MessageProcessor struct {
    contract *c.ContractInteractionInterface
    messageChannel <-chan t.Message
    errChannel chan error 
    ctx context.Context
    cancel context.CancelFunc
    messageHashes map[string][32]byte
}

func NewMessageProcessor(contract *c.ContractInteractionInterface) (*MessageProcessor, error) {
    ctx, cancel := context.WithCancel(context.Background())
    messageChannel, err := contract.Listen(ctx)
    if err != nil {
        cancel() 
        return nil, fmt.Errorf("failed to listen for messages: %v", err)
    }
    
    return &MessageProcessor{
        contract: contract,
        messageChannel: messageChannel,
        errChannel: make(chan error),
        ctx: ctx,
        cancel: cancel,
        messageHashes: make(map[string][32]byte),
    }, nil
}

func (mp *MessageProcessor) Start(){
    go func(){
        defer close(mp.errChannel)
        for {
            select {
            case msg, ok := <-mp.messageChannel:
                if !ok {
                    mp.errChannel <- fmt.Errorf("message channel closed")
                    return 
                }
                if err := mp.processMessage(msg); err != nil {
                    mp.errChannel <- err 
                    return 
                }
            case <-mp.ctx.Done():
                return
            }
        }
    }()
}

func (mp *MessageProcessor) processMessage(message t.Message) error {
    log.Printf("Processing message: %v", message)
    
    // Store the message hash for future reference
    hash := crypto.Keccak256([]byte(message.Content))

    var depHash [32]byte 
    if len(hash) != 32 {
        return fmt.Errorf("invalid hash length: %d", len(hash)) 
    }

    copy(depHash[:], hash) 

    mp.messageHashes[message.DataName] = depHash 
    
    // Check if we should reply
    if shouldReplyTo(message){
        dependencies := [][32]byte{depHash}
        err := mp.contract.Upload(
            fmt.Sprintf("Reply to %s", message.DataName),
            "ReplyOwner",
            fmt.Sprintf("Reply-%s", message.DataName),
            dependencies,
        )
        if err != nil {
            return fmt.Errorf("failed to upload reply: %v", err)
        }
    }
    
    return nil
}

func shouldReplyTo(message t.Message) bool {
    return false // Implement your logic here
}

func (mp *MessageProcessor) GetDependencyHash(dataName string) ([32]byte, bool){
    hash, ok := mp.messageHashes[dataName]
    return hash, ok
}

func (mp *MessageProcessor) Stop(){
    mp.cancel()
}

func (mp *MessageProcessor) Errors() <-chan error {
    return mp.errChannel
}
