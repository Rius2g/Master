package messageProcessor 


import (
    "context"
    "fmt"
    "log"
    "github.com/ethereum/go-ethereum/crypto"
    "time"

    c "github.com/rius2g/Master/backend/pkg/ContractInteractionInterface"
    t "github.com/rius2g/Master/backend/pkg/types"
)

type MessageProcessor struct {
    contract *c.ContractInteractionInterface
    messageChannel <-chan t.Message
    encryptedDataChannel <-chan t.EncryptedMessage
    errChannel chan error 
    ctx context.Context
    cancel context.CancelFunc
    messageHashes map[string][]byte
}

func NewMessageProcessor(contract *c.ContractInteractionInterface) (*MessageProcessor, error) {
    ctx, cancel := context.WithCancel(context.Background())

    messageChannel, encryptedChannel, err := contract.Listen(ctx)
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
        messageHashes: make(map[string][]byte),
        encryptedDataChannel: encryptedChannel,
    }, nil
}


func (mp *MessageProcessor) Start(){
    go func(){
        defer close(mp.errChannel)
        for {
            select {
            case encMessage, ok := <-mp.encryptedDataChannel:
                if !ok {
                    mp.errChannel <- fmt.Errorf("message channel closed") 
                    return 
                }
                if err := mp.processEncryptedMessage(encMessage); err != nil {
                    mp.errChannel <- err
                    return 
                }
            case decMsg, ok := <-mp.messageChannel:
                if !ok {
                    mp.errChannel <- fmt.Errorf("message channel closed")
                    return 
                }
                if err := mp.processMessage(decMsg); err != nil {
                    mp.errChannel <- err 
                    return 
                }
            case <-mp.ctx.Done():
                return
            }
        }
    }()
}


func (mp *MessageProcessor) processEncryptedMessage(message t.EncryptedMessage) error {
    hash := crypto.Keccak256(message.EncryptedData)
    mp.messageHashes[message.DataName] = hash

    if shouldReplyTo(message){
        dependencies := [][]byte{hash}

        err := mp.contract.Upload(
            fmt.Sprintf("Reploy to %s", message.DataName),
            "ReplyOwner",
            fmt.Sprintf("Reply-%s", message.DataName),
            time.Now().Add(time.Minute * 5).Unix(),
            dependencies,
            1,

        )
        if err != nil {
            return fmt.Errorf("failed to upload reply: %v", err)
        }

    }

    return nil
}

func shouldReplyTo(message t.EncryptedMessage) bool {
    return false 
}

func (mp *MessageProcessor) GetDependencyHash(dataName string) ([]byte, bool){
    hash, ok := mp.messageHashes[dataName]
    return hash, ok
}


func (mp *MessageProcessor) processMessage(message t.Message) error {
    log.Printf("Processing message: %v", message)
    return nil
}

func (mp *MessageProcessor) Stop(){
    mp.cancel()
}

func (mp *MessageProcessor) Errors() <-chan error {
    return mp.errChannel
}


