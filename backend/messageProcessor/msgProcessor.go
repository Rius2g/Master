package messageprocessor


import (
    "context"
    "fmt"
    "log"
    c "Master/ContractInteractionInterface"
    t "Master/types"
)

type MessageProcessor struct {
    contract *c.ContractInteractionInterface
    messageChannel <-chan t.Message
    errChannel chan error 
    ctx context.Context
    cancel context.CancelFunc
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
    }, nil
}


func (mp *MessageProcessor) Start(){
    go func(){
        defer close(mp.errChannel)
        for {
            select {
            case message, ok := <-mp.messageChannel:
                if !ok {
                    mp.errChannel <- fmt.Errorf("message channel closed") 
                    return 
                }
                if err := mp.processMessage(message); err != nil {
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
    return nil
}

func (mp *MessageProcessor) Stop(){
    mp.cancel()
}

func (mp *MessageProcessor) Errors() <-chan error {
    return mp.errChannel
}


