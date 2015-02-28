package inlet_http

import (
	"strconv"
	"sync"

	"github.com/gogap/errors"
	"github.com/gogap/spirit"
)

type ClassicRequester struct {
	payloadRespChans map[string]chan *spirit.Payload
	senderFactory    spirit.MessageSenderFactory
	callbackAddr     spirit.MessageAddress

	locker sync.Mutex
}

func NewClassicRequester() Requester {
	return &ClassicRequester{
		payloadRespChans: make(map[string]chan *spirit.Payload),
		senderFactory:    spirit.NewDefaultMessageSenderFactory(),
	}
}

func (p *ClassicRequester) Init(addr spirit.MessageAddress) {
	p.callbackAddr = addr
}

func (p *ClassicRequester) SetMessageSenderFactory(factory spirit.MessageSenderFactory) {
	if factory == nil {
		panic("message sender factory could not be nil")
	}
	p.senderFactory = factory
}

func (p *ClassicRequester) GetMessageSenderFactory() spirit.MessageSenderFactory {
	return p.senderFactory
}

func (p *ClassicRequester) Request(addrs []spirit.MessageAddress, payload spirit.Payload, response chan *spirit.Payload) (msgId string, err error) {
	graph := spirit.MessageGraph{}

	lenAddr := graph.AddAddress(addrs...)

	graph[strconv.Itoa(lenAddr+1)] = p.callbackAddr

	var msg spirit.ComponentMessage
	if msg, err = spirit.NewComponentMessage(graph, payload); err != nil {
		return
	}

	firstAddress := graph["1"]
	var sender spirit.MessageSender

	if sender, err = p.senderFactory.NewSender(firstAddress.Type); err != nil {
		return
	}

	msgId = msg.Id()

	p.addMessageChan(msgId, response)

	err = sender.Send(firstAddress.Url, msg)

	return
}

func (p *ClassicRequester) addMessageChan(messageId string, respChan chan *spirit.Payload) {
	p.locker.Lock()
	defer p.locker.Unlock()
	p.payloadRespChans[messageId] = respChan
}

func (p *ClassicRequester) removeMessageChan(messageId string) {
	p.locker.Lock()
	defer p.locker.Unlock()
	if _, exist := p.payloadRespChans[messageId]; exist {
		delete(p.payloadRespChans, messageId)
	}
}

func (p *ClassicRequester) getMessageChan(messageId string) (respChan chan *spirit.Payload, exist bool) {
	p.locker.Lock()
	defer p.locker.Unlock()
	respChan, exist = p.payloadRespChans[messageId]
	return
}

func (p *ClassicRequester) OnMessageReceived(payload *spirit.Payload) (interface{}, error) {
	var err error
	msgId := payload.Id()
	if msgId == "" {
		err = ERR_MESSAGE_ID_IS_EMPTY.New()
		return nil, err
	}

	var payloadChan chan *spirit.Payload

	exist := false

	if payloadChan, exist = p.getMessageChan(msgId); !exist {
		err = ERR_PAYLOAD_CHAN_NOT_EXIST.New(errors.Params{"id": msgId})
		return nil, err
	}

	payloadChan <- payload

	return nil, nil
}

func (p *ClassicRequester) OnMessageProcessed(messageId string) {
	p.removeMessageChan(messageId)
}
