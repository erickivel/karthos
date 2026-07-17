package main

import (
	"context"
	"net"
	"reflect"
	"testing"
	"time"
)

func TestProtocolTrafficUsesExactMessagesConnectionsAndBytes(t *testing.T) {
	grpcListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	grpcServer := newGRPCServer()
	go func() { _ = grpcServer.Serve(grpcListener) }()
	t.Cleanup(grpcServer.Stop)

	tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = serveTCPEcho(tcpListener) }()
	t.Cleanup(func() { _ = tcpListener.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := runTraffic(ctx, trafficConfig{
		grpcTarget:      grpcListener.Addr().String(),
		tcpTarget:       tcpListener.Addr().String(),
		unaryCalls:      20,
		streamMessages:  5,
		tcpConnections:  10,
		tcpPayloadBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := trafficResult{
		grpcRequestMessages:  25,
		grpcResponseMessages: 25,
		tcpOpened:            10,
		tcpClosed:            10,
		tcpBytesSent:         10_240,
		tcpBytesReceived:     10_240,
	}
	if !reflect.DeepEqual(want, result) {
		t.Fatalf("traffic result = %+v, want %+v", result, want)
	}
}
