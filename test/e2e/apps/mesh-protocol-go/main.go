package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	protocolServiceName = "karthos.mesh.Protocol"
	protocolUnaryMethod = "/" + protocolServiceName + "/Unary"
	protocolBidiMethod  = "/" + protocolServiceName + "/Bidi"
)

type protocolService interface {
	Unary(context.Context, *wrapperspb.BytesValue) (*wrapperspb.BytesValue, error)
	Bidi(grpc.ServerStream) error
}

type protocolServer struct{}

func (protocolServer) Unary(_ context.Context, request *wrapperspb.BytesValue) (*wrapperspb.BytesValue, error) {
	return wrapperspb.Bytes(append([]byte(nil), request.Value...)), nil
}

func (protocolServer) Bidi(stream grpc.ServerStream) error {
	for {
		request := new(wrapperspb.BytesValue)
		if err := stream.RecvMsg(request); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if err := stream.SendMsg(wrapperspb.Bytes(append([]byte(nil), request.Value...))); err != nil {
			return err
		}
	}
}

func newGRPCServer() *grpc.Server {
	server := grpc.NewServer()
	server.RegisterService(&grpc.ServiceDesc{
		ServiceName: protocolServiceName,
		HandlerType: (*protocolService)(nil),
		Methods: []grpc.MethodDesc{{
			MethodName: "Unary",
			Handler: func(srv any, ctx context.Context, decode func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
				request := new(wrapperspb.BytesValue)
				if err := decode(request); err != nil {
					return nil, err
				}
				if interceptor == nil {
					return srv.(protocolService).Unary(ctx, request)
				}
				info := &grpc.UnaryServerInfo{Server: srv, FullMethod: protocolUnaryMethod}
				handler := func(ctx context.Context, request any) (any, error) {
					return srv.(protocolService).Unary(ctx, request.(*wrapperspb.BytesValue))
				}
				return interceptor(ctx, request, info, handler)
			},
		}},
		Streams: []grpc.StreamDesc{{
			StreamName:    "Bidi",
			Handler:       func(srv any, stream grpc.ServerStream) error { return srv.(protocolService).Bidi(stream) },
			ServerStreams: true,
			ClientStreams: true,
		}},
	}, protocolServer{})
	return server
}

func serveTCPEcho(listener net.Listener) error {
	for {
		connection, err := listener.Accept()
		if err != nil {
			return err
		}
		go echoTCPConnection(connection)
	}
}

func echoTCPConnection(connection net.Conn) {
	defer func() { _ = connection.Close() }()
	buffer := make([]byte, 32*1024)
	for {
		read, err := connection.Read(buffer)
		if read > 0 {
			if _, writeErr := connection.Write(buffer[:read]); writeErr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

type trafficConfig struct {
	grpcTarget      string
	tcpTarget       string
	unaryCalls      int
	streamMessages  int
	tcpConnections  int
	tcpPayloadBytes int
}

type trafficResult struct {
	grpcRequestMessages  int
	grpcResponseMessages int
	tcpOpened            int
	tcpClosed            int
	tcpBytesSent         int
	tcpBytesReceived     int
}

func runTraffic(ctx context.Context, config trafficConfig) (trafficResult, error) {
	result := trafficResult{}
	connection, err := grpc.NewClient(config.grpcTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return result, fmt.Errorf("connect gRPC: %w", err)
	}
	defer func() { _ = connection.Close() }()

	payload := wrapperspb.Bytes([]byte("karthos-mesh-message"))
	for i := 0; i < config.unaryCalls; i++ {
		response := new(wrapperspb.BytesValue)
		if err := connection.Invoke(ctx, protocolUnaryMethod, payload, response); err != nil {
			return result, fmt.Errorf("unary call %d: %w", i+1, err)
		}
		result.grpcRequestMessages++
		result.grpcResponseMessages++
	}

	if config.streamMessages > 0 {
		stream, err := connection.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true, ClientStreams: true}, protocolBidiMethod)
		if err != nil {
			return result, fmt.Errorf("open bidi stream: %w", err)
		}
		for i := 0; i < config.streamMessages; i++ {
			if err := stream.SendMsg(payload); err != nil {
				return result, fmt.Errorf("send stream message %d: %w", i+1, err)
			}
			result.grpcRequestMessages++
			response := new(wrapperspb.BytesValue)
			if err := stream.RecvMsg(response); err != nil {
				return result, fmt.Errorf("receive stream message %d: %w", i+1, err)
			}
			result.grpcResponseMessages++
		}
		if err := stream.CloseSend(); err != nil {
			return result, fmt.Errorf("close bidi stream: %w", err)
		}
	}

	tcpPayload := make([]byte, config.tcpPayloadBytes)
	for i := range tcpPayload {
		tcpPayload[i] = byte(i % 251)
	}
	for i := 0; i < config.tcpConnections; i++ {
		connection, err := (&net.Dialer{}).DialContext(ctx, "tcp", config.tcpTarget)
		if err != nil {
			return result, fmt.Errorf("open TCP connection %d: %w", i+1, err)
		}
		result.tcpOpened++
		if _, err := connection.Write(tcpPayload); err != nil {
			_ = connection.Close()
			return result, fmt.Errorf("write TCP payload %d: %w", i+1, err)
		}
		result.tcpBytesSent += len(tcpPayload)
		response := make([]byte, len(tcpPayload))
		if _, err := io.ReadFull(connection, response); err != nil {
			_ = connection.Close()
			return result, fmt.Errorf("read TCP echo %d: %w", i+1, err)
		}
		result.tcpBytesReceived += len(response)
		if err := connection.Close(); err != nil {
			return result, fmt.Errorf("close TCP connection %d: %w", i+1, err)
		}
		result.tcpClosed++
	}
	return result, nil
}

func main() {
	mode := flag.String("mode", "server", "server or client")
	grpcAddress := flag.String("grpc-address", ":50051", "gRPC listen address")
	tcpAddress := flag.String("tcp-address", ":9000", "TCP listen address")
	grpcTarget := flag.String("grpc-target", "mesh-protocol-server:50051", "gRPC target")
	tcpTarget := flag.String("tcp-target", "mesh-protocol-server:9000", "TCP target")
	unaryCalls := flag.Int("unary", 20, "measured unary calls")
	streamMessages := flag.Int("stream-messages", 5, "measured bidi messages")
	tcpConnections := flag.Int("tcp-connections", 10, "measured TCP connections")
	tcpPayloadBytes := flag.Int("tcp-bytes", 1024, "measured payload bytes per TCP connection")
	warmupWait := flag.Duration("warmup-wait", 75*time.Second, "time between warm-up and measured traffic")
	linger := flag.Duration("linger", 2*time.Minute, "time to keep the sidecar scrapeable after measured traffic")
	flag.Parse()

	log.SetFlags(0)
	if *mode == "server" {
		if err := runServer(*grpcAddress, *tcpAddress); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *mode != "client" {
		log.Fatalf("unsupported mode %q", *mode)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	warmup, err := runTraffic(ctx, trafficConfig{
		grpcTarget: *grpcTarget, tcpTarget: *tcpTarget,
		unaryCalls: 1, streamMessages: 1, tcpConnections: 1, tcpPayloadBytes: 16,
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("WARMUP_TRAFFIC_COMPLETE %+v", warmup)
	if err := sleepContext(ctx, *warmupWait); err != nil {
		log.Fatal(err)
	}
	measured, err := runTraffic(ctx, trafficConfig{
		grpcTarget: *grpcTarget, tcpTarget: *tcpTarget,
		unaryCalls: *unaryCalls, streamMessages: *streamMessages,
		tcpConnections: *tcpConnections, tcpPayloadBytes: *tcpPayloadBytes,
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("MEASURED_TRAFFIC_COMPLETE %+v", measured)
	_ = sleepContext(ctx, *linger)
}

func runServer(grpcAddress, tcpAddress string) error {
	grpcListener, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		return fmt.Errorf("listen gRPC: %w", err)
	}
	tcpListener, err := net.Listen("tcp", tcpAddress)
	if err != nil {
		_ = grpcListener.Close()
		return fmt.Errorf("listen TCP: %w", err)
	}

	errors := make(chan error, 2)
	grpcServer := newGRPCServer()
	go func() { errors <- grpcServer.Serve(grpcListener) }()
	go func() { errors <- serveTCPEcho(tcpListener) }()
	log.Printf("mesh protocol server listening grpc=%s tcp=%s", grpcAddress, tcpAddress)
	return <-errors
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
