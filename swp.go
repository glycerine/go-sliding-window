package swp

import (
	"fmt"
	"sync"
	"time"
)

// sliding window protocol
//
// Reference: pp118-120, Computer Networks: A Systems Approach
//  by Peterson and Davie, Morgan Kaufmann Publishers, 1996.

// NB this is only sliding window, and while planned,
// doesn't have the AdvertisedWindow yet for flow-control
// and throttling the sender. See pp296-301 of Peterson and Davie.

//go:generate msgp

//msgp:ignore TxqSlot RxqSlot Semaphore SenderState RecvState SWP Session NatsNet SimNet

// Seqno is the sequence number used in the sliding window.
type Seqno int64

// Packet is what is transmitted between Sender and Recver.
type Packet struct {
	From string
	Dest string

	SeqNum    Seqno
	AckNum    Seqno
	AckOnly   bool
	KeepAlive bool

	// AvailReaderByteCap and AvailReaderMsgCap are
	// like the byte count AdvertisedWindow in TCP, but
	// since nats has both byte and message count
	// limits, we want convey these instead.
	//
	// NB we'll want to reserve some headroom in our
	// nats buffers for receipt of acks and other
	// control messages (ReservedByteCap, ReservedMsgCap)
	//
	AvailReaderBytesCap int64 // for sender throttling/flow-control
	AvailReaderMsgCap   int64 // for sender throttling/flow-control

	Data []byte
}

// TxqSlot is the sender's sliding window element.
type TxqSlot struct {
	RetryDeadline time.Time
	Pack          *Packet
}

// RxqSlot is the receiver's sliding window element.
type RxqSlot struct {
	Received bool
	Pack     *Packet
}

// SWP holds the Sliding Window Protocol state
type SWP struct {
	Sender *SenderState
	Recver *RecvState
}

// NewSWP makes a new sliding window protocol manager, holding
// both sender and receiver components.
func NewSWP(net Network, windowSize int64,
	timeout time.Duration, inbox string, destInbox string) *SWP {
	recvSz := windowSize
	sendSz := windowSize
	snd := NewSenderState(net, sendSz, timeout, inbox, destInbox)
	rcv := NewRecvState(net, recvSz, timeout, inbox, snd)
	swp := &SWP{
		Sender: snd,
		Recver: rcv,
	}
	for i := range swp.Sender.Txq {
		swp.Sender.Txq[i] = &TxqSlot{}
	}
	for i := range swp.Recver.Rxq {
		swp.Recver.Rxq[i] = &RxqSlot{}
	}

	return swp
}

// Session tracks a given point-to-point sesssion and its
// sliding window state for one of the end-points.
type Session struct {
	Swp         *SWP
	Destination string
	MyInbox     string

	Net Network
}

func NewSession(net Network,
	localInbox string,
	destInbox string,
	windowSz int64,
	timeout time.Duration) (*Session, error) {

	sess := &Session{
		Swp:         NewSWP(net, windowSz, timeout, localInbox, destInbox),
		MyInbox:     localInbox,
		Destination: destInbox,
		Net:         net,
	}
	sess.Swp.Start()

	return sess, nil
}

var ErrShutdown = fmt.Errorf("shutting down")

// Push sends a message packet, blocking until that is done.
func (sess *Session) Push(pack *Packet) {
	select {
	case sess.Swp.Sender.BlockingSend <- pack:
		p("%v Push succeeded on payload '%s' into BlockingSend", sess.MyInbox, string(pack.Data))
	case <-sess.Swp.Sender.ReqStop:
		// give up, Sender is shutting down.
	}
}

// InWindow returns true iff seqno is in [min, max].
func InWindow(seqno, min, max Seqno) bool {
	if seqno < min {
		return false
	}
	if seqno > max {
		return false
	}
	return true
}

type NatsNet struct {
	Cli *NatsClient
	mut sync.Mutex
}

func NewNatsNet(cli *NatsClient) *NatsNet {
	net := &NatsNet{Cli: cli}
	return net
}

// Network describes our network abstraction, and is implemented
// by SimNet and NatsNet.
type Network interface {

	// Send transmits the packet. It is send and pray; no
	// guarantee of delivery is made by the Network.
	Send(pack *Packet, why string) error

	// Listen starts receiving packets addressed to inbox on the returned channel.
	Listen(inbox string) (chan *Packet, error)

	// BufferCaps returns the byte and message limits
	// currently in effect, so that flow control
	// can be used to avoid sender overrunning them.
	// Not safe for concurrent access, so serialize
	// access if the Network is shared.
	BufferCaps() (bytecap int64, msgcap int64)
}

// Stop shutsdown the session
func (s *Session) Stop() {
	s.Swp.Stop()
}

// Stop the sliding window protocol
func (s *SWP) Stop() {
	s.Recver.Stop()
	s.Sender.Stop()
}

// Start the sliding window protocol
func (s *SWP) Start() {
	//q("SWP Start() called")
	s.Recver.Start()
	s.Sender.Start()
}
