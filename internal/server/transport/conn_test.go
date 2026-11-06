package transport

import (
	"bytes"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
	"wonderland-gonline/internal/protocol"
)

type signaledConn struct {
	net.Conn
	started chan struct{}
	once    sync.Once
	chunk   int
}

func (c *signaledConn) Write(p []byte) (int, error) {
	c.once.Do(func() { close(c.started) })
	if c.chunk > 0 && len(p) > c.chunk {
		p = p[:c.chunk]
	}
	return c.Conn.Write(p)
}
func await(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("transport did not make progress")
	}
}
func pipeOutput(t *testing.T, bound limits, chunk int) (*Conn, net.Conn, <-chan struct{}) {
	t.Helper()
	host, client := net.Pipe()
	signal := &signaledConn{Conn: host, started: make(chan struct{}), chunk: chunk}
	output := newConn(signal, bound, nil)
	t.Cleanup(func() { output.Close(); client.Close(); await(t, output.Done()) })
	return output, client, signal.started
}

func TestSlowReaderBoundsIncludeInFlightFrame(t *testing.T) {
	for _, tc := range []struct {
		name     string
		bound    limits
		accepted int
	}{
		{"packets", limits{2, 1024, time.Second}, 2},
		{"bytes", limits{100, 2*(protocol.FrameHeaderBytes+2) - 1, time.Second}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output, _, started := pipeOutput(t, tc.bound, 0)
			first, err := output.SendTracked([]byte{2, 1}, nil)
			if err != nil {
				t.Fatal(err)
			}
			await(t, started)
			if done, _ := first.Result(); done {
				t.Fatal("queue acceptance reported delivery")
			}
			var queued *Receipt
			for i := 1; i < tc.accepted; i++ {
				queued, err = output.SendTracked([]byte{2, 2}, nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			result := make(chan error, 1)
			go func() { result <- output.Send([]byte{2, 3}) }()
			select {
			case err := <-result:
				if !errors.Is(err, ErrBackpressure) {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("full queue waited for the slow socket")
			}
			await(t, output.Done())
			if done, err := first.Result(); !done || err == nil {
				t.Fatal("blocked write reported success", done, err)
			}
			if queued != nil {
				if done, err := queued.Result(); !done || !errors.Is(err, ErrBackpressure) {
					t.Fatal("queued receipt", done, err)
				}
			}
			if err := output.Send([]byte{2, 4}); !errors.Is(err, ErrBackpressure) {
				t.Fatal("closed queue accepted output", err)
			}
		})
	}
}

func TestFIFOImmutablePayloadPartialWritesAndDrain(t *testing.T) {
	output, client, started := pipeOutput(t, limits{64, 4096, time.Second}, 3)
	payload := []byte{20, 1, 3, 4, 5, 6}
	first, err := output.SendTracked(payload, nil)
	if err != nil {
		t.Fatal(err)
	}
	await(t, started)
	payload[2] = 99
	second, err := output.SendTracked([]byte{12, 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	output.CloseAfterFlush()
	if err := output.Send([]byte{2, 1}); !errors.Is(err, net.ErrClosed) {
		t.Fatal("draining queue accepted new data", err)
	}
	client.SetReadDeadline(time.Now().Add(time.Second))
	for _, want := range [][]byte{{20, 1, 3, 4, 5, 6}, {12, 1}} {
		got, err := protocol.Read(client)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatal("payload changed or order lost", got, err)
		}
	}
	if _, err := protocol.Read(client); !errors.Is(err, io.EOF) {
		t.Fatal("drain did not close", err)
	}
	await(t, output.Done())
	for _, r := range []*Receipt{first, second} {
		if done, err := r.Result(); !done || err != nil {
			t.Fatal("completed write", done, err)
		}
	}
}

func TestWriteTimeoutAndAbortCompleteReceipts(t *testing.T) {
	for _, abort := range []bool{false, true} {
		t.Run(map[bool]string{false: "timeout", true: "close"}[abort], func(t *testing.T) {
			output, _, started := pipeOutput(t, limits{4, 4096, 40 * time.Millisecond}, 0)
			r, err := output.SendTracked([]byte{20, 1}, nil)
			if err != nil {
				t.Fatal(err)
			}
			await(t, started)
			if abort {
				output.Close()
			}
			await(t, output.Done())
			done, err := r.Result()
			if !done || err == nil {
				t.Fatal(done, err)
			}
			if !abort {
				var timeout net.Error
				if !errors.As(err, &timeout) || !timeout.Timeout() {
					t.Fatal("write timeout lost", err)
				}
			}
		})
	}
}

func TestInvalidPayloadDoesNotCloseHealthyQueue(t *testing.T) {
	output, client, _ := pipeOutput(t, limits{4, 4096, time.Second}, 0)
	for _, p := range [][]byte{nil, make([]byte, protocol.MaxPayload+1)} {
		if err := output.Send(p); !errors.Is(err, protocol.ErrMalformed) {
			t.Fatal(err)
		}
	}
	if err := output.Send([]byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	output.CloseAfterFlush()
	client.SetReadDeadline(time.Now().Add(time.Second))
	if p, err := protocol.Read(client); err != nil || !bytes.Equal(p, []byte{12, 1}) {
		t.Fatal(p, err)
	}
	await(t, output.Done())
}

func TestConcurrentSendDrainAndClose(t *testing.T) {
	for round := 0; round < 10; round++ {
		output, client, _ := pipeOutput(t, limits{128, 16384, time.Second}, 0)
		readDone := make(chan struct{})
		go func() {
			defer close(readDone)
			for {
				if _, err := protocol.Read(client); err != nil {
					return
				}
			}
		}()
		var wg sync.WaitGroup
		for i := 0; i < 32; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, err := output.SendTracked([]byte{2, 1, 3}, nil)
				if err == nil {
					<-r.done
				}
			}()
		}
		wg.Add(2)
		go func() { defer wg.Done(); output.CloseAfterFlush() }()
		go func() { defer wg.Done(); output.Close() }()
		finished := make(chan struct{})
		go func() { wg.Wait(); close(finished) }()
		await(t, finished)
		await(t, readDone)
		await(t, output.Done())
	}
}

func TestConcurrentProducersKeepFramesAndPerProducerOrder(t *testing.T) {
	const producers, perProducer = 8, 16
	output, client, _ := pipeOutput(t, limits{256, 16384, time.Second}, 3)
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	read := make(chan error, 1)
	go func() {
		var next [producers]byte
		for i := 0; i < producers*perProducer; i++ {
			p, err := protocol.Read(client)
			if err != nil {
				read <- err
				return
			}
			if len(p) != 4 || p[0] != 2 || p[1] != 1 || p[2] >= producers || p[3] != next[p[2]] {
				read <- errors.New("concurrent output corrupted a frame or reordered one producer")
				return
			}
			next[p[2]]++
		}
		read <- nil
	}()
	var wg sync.WaitGroup
	for producer := 0; producer < producers; producer++ {
		wg.Add(1)
		go func(id byte) {
			defer wg.Done()
			for sequence := 0; sequence < perProducer; sequence++ {
				if err := output.Send([]byte{2, 1, id, byte(sequence)}); err != nil {
					t.Error(err)
					return
				}
			}
		}(byte(producer))
	}
	wg.Wait()
	output.CloseAfterFlush()
	if err := <-read; err != nil {
		t.Fatal(err)
	}
	await(t, output.Done())
}

// The writer is delayed after one completed write; the second frame must retain
// its enqueue deadline instead of receiving a fresh timeout when it starts.
func TestQueueResidenceCountsTowardWriteDeadline(t *testing.T) {
	host, client := net.Pipe()
	observed, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	output := newConn(host, limits{4, 4096, 80 * time.Millisecond}, func([]byte, error) { once.Do(func() { close(observed); <-release }) })
	t.Cleanup(func() { unblock(); output.Close(); client.Close(); await(t, output.Done()) })
	if err := output.Send([]byte{2, 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := protocol.Read(client); err != nil {
		t.Fatal(err)
	}
	await(t, observed)
	r, err := output.SendTracked([]byte{2, 2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// A paused writer simulates local scheduling/backlog, without holding any
	// queue lock or depending on a large number of queued messages.
	time.Sleep(100 * time.Millisecond)
	unblock()
	client.SetReadDeadline(time.Now().Add(time.Second))
	if p, err := protocol.Read(client); err == nil {
		t.Fatal("expired queued frame was transmitted", p)
	}
	await(t, output.Done())
	if done, err := r.Result(); !done || err == nil {
		t.Fatal(done, err)
	}
}
