package inlet_http

import (
	"sync"
	"time"

	"github.com/gogap/errors"
	"github.com/gogap/logs"
	"github.com/gogap/spirit"
)

type respChan struct {
	payloadRespChan chan spirit.Payload
	errRespChan     chan error
}

type ClassicRequester struct {
	respChans map[string]respChan

	senderFactory spirit.MessageSenderFactory

	timeout time.Duration

	locker sync.Mutex
}

func NewClassicRequester() Requester {
	return &ClassicRequester{
		respChans:     make(map[string]respChan),
		senderFactory: spirit.NewDefaultMessageSenderFactory(),
		timeout:       REQUEST_TIMEOUT,
	}
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

func (p *ClassicRequester) SetTimeout(timeout time.Duration) {
	p.timeout = timeout
}

func (p *ClassicRequester) GetTimeout() time.Duration {
	return p.timeout
}

func (p *ClassicRequester) Request(graph spirit.MessageGraph, payload spirit.Payload, payloadRespChan chan spirit.Payload, errResp chan error) (msgId string, err error) {
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

	p.addMessageChan(msgId, payloadRespChan, errResp)

	err = sender.Send(firstAddress.Url, msg)

	return
}

func (p *ClassicRequester) addMessageChan(messageId string, payloadRespChan chan spirit.Payload, errChan chan error) {
	p.locker.Lock()
	defer p.locker.Unlock()
	p.respChans[messageId] = respChan{payloadRespChan: payloadRespChan, errRespChan: errChan}
}

func (p *ClassicRequester) removeMessageChan(messageId string) {
	p.locker.Lock()
	defer p.locker.Unlock()
	if _, exist := p.respChans[messageId]; exist {
		delete(p.respChans, messageId)
	}
}

func (p *ClassicRequester) getMessageChan(messageId string) (payloadRespChan chan spirit.Payload, errRespChan chan error, exist bool) {
	p.locker.Lock()
	defer p.locker.Unlock()
	var rChan respChan
	if rChan, exist = p.respChans[messageId]; exist {
		payloadRespChan = rChan.payloadRespChan
		errRespChan = rChan.errRespChan
	}

	return
}

func (p *ClassicRequester) OnMessageReceived(payload spirit.Payload) {
	var err error
	msgId := payload.Id()
	if msgId == "" {
		err = ERR_MESSAGE_ID_IS_EMPTY.New()
		logs.Error(err)
		return
	}

	var payloadChan chan spirit.Payload

	exist := false

	if payloadChan, _, exist = p.getMessageChan(msgId); !exist {
		err = ERR_PAYLOAD_CHAN_NOT_EXIST.New(errors.Params{"id": msgId})
		logs.Error(err)
		return
	}

	select {
	case payloadChan <- payload:
	case <-time.After(time.Duration(p.timeout)):
	}

	return
}

func (p *ClassicRequester) OnMessageError(payload spirit.Payload) {
	var err error

	msgId := payload.Id()
	if msgId == "" {
		err = ERR_MESSAGE_ID_IS_EMPTY.New()
		logs.Error(err)
		return
	}

	var errRespChan chan error

	exist := false

	if _, errRespChan, exist = p.getMessageChan(msgId); !exist {
		err = ERR_PAYLOAD_CHAN_NOT_EXIST.New(errors.Params{"id": msgId})
		logs.Error(err)
		return
	}

	select {
	case errRespChan <- err:
	case <-time.After(time.Duration(p.timeout)):
	}

	return
}

func (p *ClassicRequester) OnMessageProcessed(messageId string) {
	p.removeMessageChan(messageId)
}
