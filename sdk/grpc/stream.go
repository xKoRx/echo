package grpc

import (
	"context"
	"fmt"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// StreamClient abstracción para cliente de streaming bidireccional.
//
// Facilita el manejo de send/receive con channels y serialización de writes.
type StreamClient struct {
	stream grpc.ClientStream
	sendCh chan proto.Message
	recvCh chan proto.Message
	errCh  chan error
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	closed bool
}

// NewStreamClient crea un nuevo StreamClient.
//
// Example:
//
//	stream, err := pb.NewAgentServiceClient(conn).StreamBidi(ctx)
//	streamClient := grpc.NewStreamClient(stream)
//	defer streamClient.Close()
//
//	// Enviar
//	streamClient.Send(&pb.AgentMessage{...})
//
//	// Recibir
//	for msg := range streamClient.Receive() {
//	    // Procesar mensaje
//	}
func NewStreamClient(stream grpc.ClientStream) *StreamClient {
	ctx, cancel := context.WithCancel(stream.Context())

	sc := &StreamClient{
		stream: stream,
		sendCh: make(chan proto.Message, 100),
		recvCh: make(chan proto.Message, 100),
		errCh:  make(chan error, 1),
		ctx:    ctx,
		cancel: cancel,
	}

	// Goroutine de envío (serializa writes)
	go sc.sendLoop()

	// Goroutine de recepción
	go sc.recvLoop()

	return sc
}

// sendLoop maneja el envío serializado de mensajes.
func (sc *StreamClient) sendLoop() {
	for {
		select {
		case msg := <-sc.sendCh:
			if err := sc.stream.SendMsg(msg); err != nil {
				sc.errCh <- fmt.Errorf("send error: %w", err)
				return
			}
		case <-sc.ctx.Done():
			return
		}
	}
}

// recvLoop maneja la recepción de mensajes.
func (sc *StreamClient) recvLoop() {
	for {
		var msg proto.Message
		// Aquí asumimos que el mensaje concreto se proporciona al llamar Receive
		// En la práctica, esto requiere un tipo específico
		// Por ahora, usamos interface{} y el caller hace type assertion
		if err := sc.stream.RecvMsg(&msg); err != nil {
			sc.errCh <- fmt.Errorf("recv error: %w", err)
			close(sc.recvCh)
			return
		}

		select {
		case sc.recvCh <- msg:
		case <-sc.ctx.Done():
			return
		}
	}
}

// Send envía un mensaje por el stream.
//
// Es thread-safe (serializa internamente).
func (sc *StreamClient) Send(msg proto.Message) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.closed {
		return fmt.Errorf("stream is closed")
	}

	select {
	case sc.sendCh <- msg:
		return nil
	case <-sc.ctx.Done():
		return sc.ctx.Err()
	}
}

// Receive retorna un channel de mensajes recibidos.
//
// El channel se cierra cuando el stream termina.
func (sc *StreamClient) Receive() <-chan proto.Message {
	return sc.recvCh
}

// Errors retorna un channel de errores.
//
// Solo se envía un error (el primero que ocurre).
func (sc *StreamClient) Errors() <-chan error {
	return sc.errCh
}

// Close cierra el stream.
func (sc *StreamClient) Close() error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.closed {
		return nil
	}

	sc.closed = true
	sc.cancel()
	close(sc.sendCh)

	return nil
}

// Context retorna el contexto del stream.
func (sc *StreamClient) Context() context.Context {
	return sc.ctx
}

// StreamServer abstracción para servidor de streaming bidireccional.
//
// Similar a StreamClient pero para el lado del servidor.
type StreamServer struct {
	stream grpc.ServerStream
	sendCh chan proto.Message
	recvCh chan proto.Message
	errCh  chan error
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	closed bool
}

// NewStreamServer crea un nuevo StreamServer.
//
// Example (en handler de servicio):
//
//	func (s *myService) StreamBidi(stream pb.AgentService_StreamBidiServer) error {
//	    streamServer := grpc.NewStreamServer(stream)
//	    defer streamServer.Close()
//
//	    for msg := range streamServer.Receive() {
//	        // Procesar y responder
//	        streamServer.Send(response)
//	    }
//
//	    return nil
//	}
func NewStreamServer(stream grpc.ServerStream) *StreamServer {
	ctx, cancel := context.WithCancel(stream.Context())

	ss := &StreamServer{
		stream: stream,
		sendCh: make(chan proto.Message, 100),
		recvCh: make(chan proto.Message, 100),
		errCh:  make(chan error, 1),
		ctx:    ctx,
		cancel: cancel,
	}

	// Goroutine de envío
	go ss.sendLoop()

	// Goroutine de recepción
	go ss.recvLoop()

	return ss
}

// sendLoop maneja el envío serializado.
func (ss *StreamServer) sendLoop() {
	for {
		select {
		case msg := <-ss.sendCh:
			if err := ss.stream.SendMsg(msg); err != nil {
				ss.errCh <- fmt.Errorf("send error: %w", err)
				return
			}
		case <-ss.ctx.Done():
			return
		}
	}
}

// recvLoop maneja la recepción.
func (ss *StreamServer) recvLoop() {
	for {
		var msg proto.Message
		if err := ss.stream.RecvMsg(&msg); err != nil {
			ss.errCh <- fmt.Errorf("recv error: %w", err)
			close(ss.recvCh)
			return
		}

		select {
		case ss.recvCh <- msg:
		case <-ss.ctx.Done():
			return
		}
	}
}

// Send envía un mensaje por el stream.
func (ss *StreamServer) Send(msg proto.Message) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if ss.closed {
		return fmt.Errorf("stream is closed")
	}

	select {
	case ss.sendCh <- msg:
		return nil
	case <-ss.ctx.Done():
		return ss.ctx.Err()
	}
}

// Receive retorna un channel de mensajes recibidos.
func (ss *StreamServer) Receive() <-chan proto.Message {
	return ss.recvCh
}

// Errors retorna un channel de errores.
func (ss *StreamServer) Errors() <-chan error {
	return ss.errCh
}

// Close cierra el stream.
func (ss *StreamServer) Close() error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if ss.closed {
		return nil
	}

	ss.closed = true
	ss.cancel()
	close(ss.sendCh)

	return nil
}

// Context retorna el contexto del stream.
func (ss *StreamServer) Context() context.Context {
	return ss.ctx
}

// StreamHelper funciones helper para streams.
type StreamHelper struct{}

// SendAll envía múltiples mensajes por un stream.
//
// Example:
//
//	messages := []proto.Message{msg1, msg2, msg3}
//	if err := grpc.StreamHelper{}.SendAll(streamClient, messages); err != nil {
//	    return err
//	}
func (sh StreamHelper) SendAll(stream interface{ Send(proto.Message) error }, messages []proto.Message) error {
	for i, msg := range messages {
		if err := stream.Send(msg); err != nil {
			return fmt.Errorf("failed to send message %d: %w", i, err)
		}
	}
	return nil
}

// ReceiveAll recibe todos los mensajes hasta que el stream se cierre.
//
// NOTA: Esta función bloquea hasta que el stream se cierre.
func (sh StreamHelper) ReceiveAll(recvCh <-chan proto.Message) []proto.Message {
	var messages []proto.Message
	for msg := range recvCh {
		messages = append(messages, msg)
	}
	return messages
}

// DrainErrors drena el channel de errores y retorna el primero.
func (sh StreamHelper) DrainErrors(errCh <-chan error) error {
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

