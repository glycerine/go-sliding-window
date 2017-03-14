package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/glycerine/bchan"
	"github.com/glycerine/go-sliding-window"
)

func main() {

	host := os.Getenv("BROKER_HOST")
	port := os.Getenv("BROKER_PORT")
	if host == "" {
		fmt.Fprintf(os.Stderr, "BROKER_HOST in env was not set. Setting required.\n")
		os.Exit(1)
	}
	if port == "" {
		fmt.Fprintf(os.Stderr, "BROKER_PORT in env was not set. Setting required.\n")
		os.Exit(1)
	}
	nport, err := strconv.Atoi(port)
	panicOn(err)

	fmt.Printf("contacting nats://%v:%v\n", host, port)

	// ===============================
	// setup nats client for a publisher
	// ===============================

	skipTLS := true
	asyncErrCrash := false
	pubC := swp.NewNatsClientConfig(host, nport, "A", "A", skipTLS, asyncErrCrash)
	pub := swp.NewNatsClient(pubC)
	err = pub.Start()
	panicOn(err)
	defer pub.Close()

	// ===============================
	// make a session for each
	// ===============================

	anet := swp.NewNatsNet(pub)

	//fmt.Printf("pub = %#v\n", pub)

	to := time.Millisecond * 100
	A, err := swp.NewSession(swp.SessionConfig{Net: anet, LocalInbox: "A", DestInbox: "B",
		WindowMsgSz: 1000, WindowByteSz: -1, Timeout: to, Clk: swp.RealClk})
	panicOn(err)

	//rep := swp.ReportOnSubscription(pub.Scrip)
	//fmt.Printf("rep = %#v\n", rep)

	msgLimit := int64(1000)
	bytesLimit := int64(600000)
	A.Swp.Sender.FlowCt = &swp.FlowCtrl{Flow: swp.Flow{
		ReservedByteCap: 600000,
		ReservedMsgCap:  1000,
	}}
	swp.SetSubscriptionLimits(pub.Scrip, msgLimit, bytesLimit)

	A.SelfConsumeForTesting() // read any acks

	// writer does:
	ca := bchan.New(1)
	A.Push(&swp.Packet{
		From: "A",
		Dest: "B",
		Data: []byte("hello world"),

		CliAcked: ca,
	})
	// wait for receiver to ack it.
	<-ca.Ch
	fmt.Printf("\nwe got end-to-end ack from receiver that packet was delivered\n")
	ca.BcastAck()

	// reader does: <-A.ReadMessagesCh
	A.Stop()
}

func panicOn(err error) {
	if err != nil {
		panic(err)
	}
}