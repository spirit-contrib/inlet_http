package inlet_http

import (
	"sync"
	"time"

	"github.com/gogap/errors"
	"github.com/gogap/logs"
	"github.com/gogap/spirit"
	"github.com/spirit-contrib/receivers/localchan"
)

var (
	DefaultMessageChanSize = 1000
)

type respChan struct {
	payloadRespChan chan spirit.Payload
	errRespChan     chan error
}

type ClassicRequester struct {
	respChans map[string]respChan

	messageChan chan spirit.ComponentMessage

	timeout time.Duration

	locker sync.Mutex
}

func NewClassicRequester() Requester {
	messageChan := make(chan spirit.ComponentMessage, DefaultMessageChanSize)

	localchan.RegisterMessageChan("localchan://inlet_http/empty/nothing", messageChan)

	return &ClassicRequester{
		messageChan: messageChan,
		respChans:   make(map[string]respChan),
		timeout:     REQUEST_TIMEOUT,
	}
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

	graph.AddAddressToHead(spirit.MessageAddress{"localchan", "localchan://inlet_http/empty/nothing"})

	msgId = msg.Id()

	p.addMessageChan(msgId, payloadRespChan, errResp)

	p.messageChan <- msg

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
