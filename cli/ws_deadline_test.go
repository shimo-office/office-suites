package main

import (
	"bufio"
	"net"
	"testing"
	"time"
)

func TestWSCallBoundsBlockedWritesAndDiscardsConnection(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	defer client.Close()
	ws := &WSConn{conn: client, rd: bufio.NewReader(client)}
	done := make(chan error, 1)
	go func() {
		_, err := ws.Call(1, "Runtime.evaluate", map[string]any{"expression": "mutation()"}, 30*time.Millisecond)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("blocked write succeeded")
		}
	case <-time.After(500 * time.Millisecond):
		client.Close()
		<-done
		t.Fatal("RPC write exceeded total deadline")
	}
	started := time.Now()
	if _, err := ws.Call(2, "Runtime.evaluate", nil, time.Second); err == nil {
		t.Fatal("timed-out connection reused")
	}
	if time.Since(started) > 100*time.Millisecond {
		t.Fatal("failed connection remained open")
	}
}

func TestWSCallBoundsPongWrite(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	defer client.Close()
	go func() {
		peer := &WSConn{conn: server, rd: bufio.NewReader(server)}
		if _, err := peer.readMessage(); err != nil {
			return
		}
		_, _ = server.Write([]byte{0x89, 0x00})
	}()
	ws := &WSConn{conn: client, rd: bufio.NewReader(client)}
	done := make(chan error, 1)
	go func() { _, err := ws.Call(1, "Runtime.evaluate", nil, 30*time.Millisecond); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("unanswered RPC succeeded")
		}
	case <-time.After(500 * time.Millisecond):
		client.Close()
		<-done
		t.Fatal("pong write exceeded RPC deadline")
	}
}

func TestWSCallDiscardsPartiallyReadFrame(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	defer client.Close()
	go func() {
		peer := &WSConn{conn: server, rd: bufio.NewReader(server)}
		if _, err := peer.readMessage(); err != nil {
			return
		}
		_, _ = server.Write([]byte{0x81})
	}()
	ws := &WSConn{conn: client, rd: bufio.NewReader(client)}
	if _, err := ws.Call(1, "Runtime.evaluate", nil, 30*time.Millisecond); err == nil {
		t.Fatal("partial frame succeeded")
	}
	done := make(chan error, 1)
	go func() { _, err := ws.Call(2, "Runtime.evaluate", nil, time.Second); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("partial-frame connection reused")
		}
	case <-time.After(150 * time.Millisecond):
		client.Close()
		<-done
		t.Fatal("partial frame left the connection reusable")
	}
}
