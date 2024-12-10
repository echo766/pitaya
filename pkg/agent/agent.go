// Copyright (c) nano Author and TFG Co. All Rights Reserved.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package agent

import (
	"context"
	gojson "encoding/json"
	e "errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/echo766/pitaya/pkg/async"
	"github.com/echo766/pitaya/pkg/conn/codec"
	"github.com/echo766/pitaya/pkg/conn/message"
	"github.com/echo766/pitaya/pkg/conn/packet"
	"github.com/echo766/pitaya/pkg/constants"
	"github.com/echo766/pitaya/pkg/errors"
	"github.com/echo766/pitaya/pkg/logger"
	"github.com/echo766/pitaya/pkg/metrics"
	"github.com/echo766/pitaya/pkg/protos"
	"github.com/echo766/pitaya/pkg/serialize"
	"github.com/echo766/pitaya/pkg/session"
	"github.com/echo766/pitaya/pkg/tracing"
	"github.com/echo766/pitaya/pkg/util"

	opentracing "github.com/opentracing/opentracing-go"
)

const handlerType = "handler"

type (
	agentImpl struct {
		Session            session.Session // session
		sessionPool        session.SessionPool
		appDieChan         chan bool         // app die channel
		chDie              chan struct{}     // wait for close
		chSend             chan pendingWrite // push message queue
		chStopHeartbeat    chan struct{}     // stop heartbeats
		chStopWrite        chan struct{}     // stop writing messages
		closeMutex         sync.Mutex
		conn               net.Conn            // low-level conn fd
		decoder            codec.PacketDecoder // binary decoder
		encoder            codec.PacketEncoder // binary encoder
		heartbeatTimeout   time.Duration
		lastAt             int64 // last heartbeat unix time stamp
		messageEncoder     message.Encoder
		messagesBufferSize int // size of the pending messages buffer
		metricsReporters   []metrics.Reporter
		serializer         serialize.Serializer // message serializer
		state              int32                // current agent state

		chStopHandle chan struct{} // stop handle
		chMessages   chan interface{}
	}

	pendingMessage struct {
		ctx     context.Context
		typ     message.Type // message type
		route   string       // message route (push)
		mid     uint         // response message id (response)
		payload interface{}  // payload
		err     bool         // if its an error message
	}

	pendingWrite struct {
		ctx  context.Context
		data []byte
		err  error
	}

	// Agent corresponds to a user and is used for storing raw Conn information
	Agent interface {
		GetSession() session.Session
		Push(route string, v interface{}) error
		ResponseMID(ctx context.Context, mid uint, v interface{}, isError ...bool) error
		Close() error
		RemoteAddr() net.Addr
		String() string
		GetStatus() int32
		Kick(ctx context.Context) error
		SetLastAt()
		SetStatus(state int32)
		IPVersion() string
		SendHandshakeResponse(code int) error
		SendRequest(ctx context.Context, serverID, route string, v interface{}) (*protos.Response, error)
		AnswerWithError(ctx context.Context, mid uint, err error)
		OnHeartbeat()

		Handle(dispatch func(interface{}))
		PushUnhandleMessage(ctx context.Context, v interface{}) error
	}

	// AgentFactory factory for creating Agent instances
	AgentFactory interface {
		CreateAgent(conn net.Conn) Agent
	}

	agentFactoryImpl struct {
		sessionPool        session.SessionPool
		appDieChan         chan bool           // app die channel
		decoder            codec.PacketDecoder // binary decoder
		encoder            codec.PacketEncoder // binary encoder
		heartbeatTimeout   time.Duration
		messageEncoder     message.Encoder
		messagesBufferSize int // size of the pending messages buffer
		metricsReporters   []metrics.Reporter
		serializer         serialize.Serializer // message serializer
	}
)

// NewAgentFactory ctor
func NewAgentFactory(
	appDieChan chan bool,
	decoder codec.PacketDecoder,
	encoder codec.PacketEncoder,
	serializer serialize.Serializer,
	heartbeatTimeout time.Duration,
	messageEncoder message.Encoder,
	messagesBufferSize int,
	sessionPool session.SessionPool,
	metricsReporters []metrics.Reporter,
) AgentFactory {
	return &agentFactoryImpl{
		appDieChan:         appDieChan,
		decoder:            decoder,
		encoder:            encoder,
		heartbeatTimeout:   heartbeatTimeout,
		messageEncoder:     messageEncoder,
		messagesBufferSize: messagesBufferSize,
		sessionPool:        sessionPool,
		metricsReporters:   metricsReporters,
		serializer:         serializer,
	}
}

// CreateAgent returns a new agent
func (f *agentFactoryImpl) CreateAgent(conn net.Conn) Agent {
	return newAgent(conn, f.decoder, f.encoder, f.serializer, f.heartbeatTimeout, f.messagesBufferSize, f.appDieChan, f.messageEncoder, f.metricsReporters, f.sessionPool)
}

// NewAgent create new agent instance
func newAgent(
	conn net.Conn,
	packetDecoder codec.PacketDecoder,
	packetEncoder codec.PacketEncoder,
	serializer serialize.Serializer,
	heartbeatTime time.Duration,
	messagesBufferSize int,
	dieChan chan bool,
	messageEncoder message.Encoder,
	metricsReporters []metrics.Reporter,
	sessionPool session.SessionPool,
) Agent {
	a := &agentImpl{
		appDieChan:         dieChan,
		chDie:              make(chan struct{}),
		chSend:             make(chan pendingWrite, messagesBufferSize),
		chStopHeartbeat:    make(chan struct{}),
		chStopWrite:        make(chan struct{}),
		messagesBufferSize: messagesBufferSize,
		conn:               conn,
		decoder:            packetDecoder,
		encoder:            packetEncoder,
		heartbeatTimeout:   heartbeatTime,
		lastAt:             time.Now().Unix(),
		serializer:         serializer,
		state:              constants.StatusStart,
		messageEncoder:     messageEncoder,
		metricsReporters:   metricsReporters,
		sessionPool:        sessionPool,
		chMessages:         make(chan interface{}, 5),
		chStopHandle:       make(chan struct{}),
	}

	// binding session
	s := sessionPool.NewSession(a, true)
	metrics.ReportNumberOfConnectedClients(metricsReporters, sessionPool.GetSessionCount())
	a.Session = s
	return a
}

func (a *agentImpl) getMessageFromPendingMessage(pm pendingMessage) (*message.Message, error) {
	payload, err := util.SerializeOrRaw(a.serializer, pm.payload)
	if err != nil {
		payload, err = util.GetErrorPayload(a.serializer, err)
		if err != nil {
			return nil, err
		}
	}

	// construct message and encode
	m := &message.Message{
		Type:  pm.typ,
		Data:  payload,
		Route: pm.route,
		ID:    pm.mid,
		Err:   pm.err,
	}

	return m, nil
}

func (a *agentImpl) packetEncodeMessage(m *message.Message) ([]byte, error) {
	em, err := a.messageEncoder.Encode(m)
	if err != nil {
		return nil, err
	}

	// packet encode
	p, err := a.encoder.Encode(packet.Data, em)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (a *agentImpl) send(pendingMsg pendingMessage) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = errors.NewError(constants.ErrBrokenPipe, errors.ErrClientClosedRequest)
		}
	}()
	a.reportChannelSize()

	m, err := a.getMessageFromPendingMessage(pendingMsg)
	if err != nil {
		return err
	}

	// packet encode
	p, err := a.packetEncodeMessage(m)
	if err != nil {
		return err
	}

	pWrite := pendingWrite{
		ctx:  pendingMsg.ctx,
		data: p,
	}

	if pendingMsg.err {
		pWrite.err = util.GetErrorFromPayload(a.serializer, m.Data)
	}

	// don't block if the channel is full, just drop the message
	if len(a.chSend) > a.messagesBufferSize {
		logger.Log.WithFields(logger.Fields{
			"uid": a.Session.UID(),
			"len": len(a.chSend),
		}).Error("Send message to client failed. Channel is full")
		return constants.ErrAgentSendChannelFull
	}

	// chSend is never closed so we need this to don't block if agent is already closed
	select {
	case a.chSend <- pWrite:
	case <-a.chDie:
	}
	return
}

// GetSession returns the agent session
func (a *agentImpl) GetSession() session.Session {
	return a.Session
}

// Push implementation for NetworkEntity interface
func (a *agentImpl) Push(route string, v interface{}) error {
	if a.GetStatus() == constants.StatusClosed {
		return errors.NewError(constants.ErrBrokenPipe, errors.ErrClientClosedRequest)
	}

	switch d := v.(type) {
	case []byte:
		logger.Log.Debugf("Type=Push, ID=%d, UID=%s, Route=%s, Data=%dbytes",
			a.Session.ID(), a.Session.UID(), route, len(d))
	default:
		logger.Log.Debugf("Type=Push, ID=%d, UID=%s, Route=%s, Data=%+v",
			a.Session.ID(), a.Session.UID(), route, v)
	}
	return a.send(pendingMessage{typ: message.Push, route: route, payload: v})
}

// ResponseMID implementation for NetworkEntity interface
// Respond message to session
func (a *agentImpl) ResponseMID(ctx context.Context, mid uint, v interface{}, isError ...bool) error {
	err := false
	if len(isError) > 0 {
		err = isError[0]
	}
	if a.GetStatus() == constants.StatusClosed {
		return errors.NewError(constants.ErrBrokenPipe, errors.ErrClientClosedRequest)
	}

	if mid <= 0 {
		return constants.ErrSessionOnNotify
	}

	switch d := v.(type) {
	case []byte:
		logger.Log.Debugf("Type=Response, ID=%d, UID=%s, MID=%d, Data=%dbytes",
			a.Session.ID(), a.Session.UID(), mid, len(d))
	default:
		logger.Log.Infof("Type=Response, ID=%d, UID=%s, MID=%d, Data=%+v",
			a.Session.ID(), a.Session.UID(), mid, v)
	}

	return a.send(pendingMessage{ctx: ctx, typ: message.Response, mid: mid, payload: v, err: err})
}

// Close closes the agent, cleans inner state and closes low-level connection.
// Any blocked Read or Write operations will be unblocked and return errors.
func (a *agentImpl) Close() error {
	a.closeMutex.Lock()
	defer a.closeMutex.Unlock()
	if a.GetStatus() == constants.StatusClosed {
		return constants.ErrCloseClosedSession
	}
	a.SetStatus(constants.StatusClosed)

	logger.Log.Debugf("Session closed, ID=%d, UID=%s, IP=%s",
		a.Session.ID(), a.Session.UID(), a.conn.RemoteAddr())

	// prevent closing closed channel
	select {
	case <-a.chDie:
		// expect
	default:
		close(a.chStopHandle)
		close(a.chStopWrite)
		close(a.chStopHeartbeat)
		close(a.chDie)
		a.onSessionClosed(a.Session)
	}

	metrics.ReportNumberOfConnectedClients(a.metricsReporters, a.sessionPool.GetSessionCount())

	return a.conn.Close()
}

// RemoteAddr implementation for NetworkEntity interface
// returns the remote network address.
func (a *agentImpl) RemoteAddr() net.Addr {
	return a.conn.RemoteAddr()
}

// String, implementation for Stringer interface
func (a *agentImpl) String() string {
	return fmt.Sprintf("Remote=%s, LastTime=%d", a.conn.RemoteAddr().String(), atomic.LoadInt64(&a.lastAt))
}

// GetStatus gets the status
func (a *agentImpl) GetStatus() int32 {
	return atomic.LoadInt32(&a.state)
}

// Kick sends a kick packet to a client
func (a *agentImpl) Kick(ctx context.Context) error {
	// packet encode
	p, err := a.encoder.Encode(packet.Kick, nil)
	if err != nil {
		return err
	}
	_, err = a.conn.Write(p)
	return err
}

// SetLastAt sets the last at to now
func (a *agentImpl) SetLastAt() {
	atomic.StoreInt64(&a.lastAt, time.Now().Unix())
}

// SetStatus sets the agent status
func (a *agentImpl) SetStatus(state int32) {
	atomic.StoreInt32(&a.state, state)
}

// Handle handles the messages from and to a client
func (a *agentImpl) Handle(dispatch func(interface{})) {
	defer func() {
		a.Close()
		logger.Log.Debugf("Session handle goroutine exit, SessionID=%d, UID=%s", a.Session.ID(), a.Session.UID())
	}()

	async.GoRaw(func() {
		a.write()
	})
	async.GoRaw(func() {
		a.heartbeat()
	})

	async.GoRaw(func() {
		a.handle(dispatch)
	})

	<-a.chDie // agent closed signal
}

func (a *agentImpl) handle(dispatch func(interface{})) {
	defer func() {
		a.Close()
		logger.Log.Debugf("Session handle goroutine exit, SessionID=%d, UID=%s", a.Session.ID(), a.Session.UID())
	}()

	for {
		select {
		case msg := <-a.chMessages:
			dispatch(msg)
		case <-a.chStopHandle:
			return
		}
	}
}

// IPVersion returns the remote address ip version.
// net.TCPAddr and net.UDPAddr implementations of String()
// always construct result as <ip>:<port> on both
// ipv4 and ipv6. Also, to see if the ip is ipv6 they both
// check if there is a colon on the string.
// So checking if there are more than one colon here is safe.
func (a *agentImpl) IPVersion() string {
	version := constants.IPv4

	ipPort := a.RemoteAddr().String()
	if strings.Count(ipPort, ":") > 1 {
		version = constants.IPv6
	}

	return version
}

func (a *agentImpl) heartbeat() {
	ticker := time.NewTicker(a.heartbeatTimeout)

	defer func() {
		ticker.Stop()
		a.Close()
	}()

	for {
		select {
		case <-ticker.C:
			deadline := time.Now().Add(-2 * a.heartbeatTimeout).Unix()
			if atomic.LoadInt64(&a.lastAt) < deadline {
				logger.Log.Debugf("Session heartbeat timeout, LastTime=%d, Deadline=%d", atomic.LoadInt64(&a.lastAt), deadline)
				return
			}
		case <-a.chDie:
			return
		case <-a.chStopHeartbeat:
			return
		}
	}
}

func (a *agentImpl) onSessionClosed(s session.Session) {
	defer func() {
		if err := recover(); err != nil {
			logger.Log.Errorf("pitaya/onSessionClosed: %v", err)
		}
	}()

	for _, fn1 := range s.GetOnCloseCallbacks() {
		fn1()
	}

	for _, fn2 := range a.sessionPool.GetSessionCloseCallbacks() {
		fn2(s)
	}
}

func (a *agentImpl) OnHeartbeat() {
	defer func() {
		if err := recover(); err != nil {
			logger.Log.Errorf("pitaya/onSessionHeartbeat: %v", err)
		}
	}()

	// chSend is never closed so we need this to don't block if agent is already closed
	select {
	case a.chSend <- pendingWrite{data: heartbeatEncode(a.encoder)}:
	case <-a.chDie:
		return
	case <-a.chStopHeartbeat:
		return
	}

	for _, fn2 := range a.sessionPool.GetSessionHeartbeatCallbacks() {
		fn2(a.Session)
	}
}

// SendHandshakeResponse sends a handshake response
func (a *agentImpl) SendHandshakeResponse(code int) error {
	_, err := a.conn.Write(handshakeEncode(code, a.heartbeatTimeout, a.encoder))
	return err
}

func (a *agentImpl) write() {
	// clean func
	defer func() {
		a.Close()
	}()

	for {
		select {
		case pWrite := <-a.chSend:
			// close agent if low-level Conn broken
			if _, err := a.conn.Write(pWrite.data); err != nil {
				tracing.FinishSpan(pWrite.ctx, err)
				metrics.ReportTimingFromCtx(pWrite.ctx, a.metricsReporters, handlerType, err)
				logger.Log.Errorf("Failed to write in conn: %s", err.Error())
				return
			}
			var e error
			tracing.FinishSpan(pWrite.ctx, e)
			metrics.ReportTimingFromCtx(pWrite.ctx, a.metricsReporters, handlerType, pWrite.err)
		case <-a.chStopWrite:
			return
		}
	}
}

// SendRequest sends a request to a server
func (a *agentImpl) SendRequest(ctx context.Context, serverID, route string, v interface{}) (*protos.Response, error) {
	return nil, e.New("not implemented")
}

// AnswerWithError answers with an error
func (a *agentImpl) AnswerWithError(ctx context.Context, mid uint, err error) {
	var e error
	defer func() {
		if e != nil {
			tracing.FinishSpan(ctx, e)
			metrics.ReportTimingFromCtx(ctx, a.metricsReporters, handlerType, e)
		}
	}()
	if ctx != nil && err != nil {
		s := opentracing.SpanFromContext(ctx)
		if s != nil {
			tracing.LogError(s, err.Error())
		}
	}
	p, e := util.GetErrorPayload(a.serializer, err)
	if e != nil {
		logger.Log.Errorf("error answering the user with an error: %s", e.Error())
		return
	}
	e = a.Session.ResponseMID(ctx, mid, p, true)
	if e != nil {
		logger.Log.Errorf("error answering the user with an error: %s", e.Error())
	}
}

func handshakeEncode(code int, heartbeatTimeout time.Duration, packetEncoder codec.PacketEncoder) []byte {
	hData := map[string]interface{}{
		"code": code,
		"sys": map[string]interface{}{
			"heartbeat": heartbeatTimeout.Seconds(),
			"t":         time.Now().Unix(),
		},
	}
	data, _ := gojson.Marshal(hData)
	hrd, _ := packetEncoder.Encode(packet.Handshake, data)
	return hrd
}

func heartbeatEncode(packetEncoder codec.PacketEncoder) []byte {
	hData := map[string]interface{}{
		"t": time.Now().Unix(),
	}
	data, _ := gojson.Marshal(hData)
	hbd, _ := packetEncoder.Encode(packet.Heartbeat, data)
	return hbd
}

func (a *agentImpl) reportChannelSize() {
	chSendCapacity := a.messagesBufferSize - len(a.chSend)
	if chSendCapacity == 0 {
		logger.Log.Warnf("chSend is at maximum capacity")
	}
	for _, mr := range a.metricsReporters {
		if err := mr.ReportGauge(metrics.ChannelCapacity, map[string]string{"channel": "agent_chsend"}, float64(chSendCapacity)); err != nil {
			logger.Log.Warnf("failed to report chSend channel capaacity: %s", err.Error())
		}
	}
}

func (a *agentImpl) PushUnhandleMessage(ctx context.Context, v interface{}) error {
	select {
	case a.chMessages <- v:
	case <-ctx.Done():
		return constants.ErrHandleReqTimeout
	case <-a.chDie:
	}

	return nil
}
