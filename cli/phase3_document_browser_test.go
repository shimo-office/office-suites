package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPhase3PresenceObserver(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("optional JavaScript observer test requires existing Node runtime")
	}
	setup := `global.window=global;class Socket{static OPEN=1;constructor(){this.readyState=1;this.handlers={}}addEventListener(n,f){(this.handlers[n]??=[]).push(f)}fire(n,data){for(const f of this.handlers[n]||[])f({data})}};global.WebSocket=Socket;global.XMLHttpRequest=class{send(){}addEventListener(){}};`
	cases := `
 const assert=(v,m)=>{if(!v)throw Error(m)};const check=()=>window.__shimoPhase3Presence();const ws=new WebSocket();
 const send=(s,type,data)=>s.fire("message",'42'+JSON.stringify(["message",{type:"COLLABROOM",data:{type,...data}}]));
 assert(!check().ready,"default empty list must not pass");
 send(ws,"RECONNECT_READY",{clients:[]});assert(!check().ready,"unbound list must not pass");
 send(ws,"RECONNECT_READY",{fileGuid:"other",clients:[]});assert(!check().ready,"wrong room must not pass");
 send(ws,"RECONNECT_READY",{fileGuid:"target",clients:[{id:1}]});assert(check().ready&&check().connected&&check().guid==="target","authoritative init failed");
 send(ws,"USER_ENTER",{fileGuid:"target",userInfo:{id:2}});assert(check().ids.includes("2"),"enter not tracked");
 send(ws,"USER_LEAVE",{fileGuid:"other",userInfo:{id:2}});assert(check().ids.includes("2"),"foreign room leave removed target user");
 send(ws,"USER_LEAVE",{userInfo:{id:2}});assert(!check().ids.includes("2"),"bound socket leave not tracked");
 const other=new WebSocket();other.fire("close");assert(check().connected,"unrelated socket closure invalidated target");
 ws.readyState=3;assert(!check().connected,"dead target socket must not pass");ws.readyState=1;
 send(ws,"RECONNECT_READY",{fileGuid:"target",clients:[{}]});assert(!check().ready,"malformed user must fail closed");
 send(ws,"RECONNECT_READY",{fileGuid:"target",clients:[]});assert(check().ready,"known empty list should initialize");
 ws.fire("close");assert(!check().ready&&!check().connected,"close must invalidate");
 const current=new WebSocket();send(current,"RECONNECT_READY",{fileGuid:"target",clients:[{id:1},{id:2}]});
 send(ws,"USER_LEAVE",{fileGuid:"target",userInfo:{id:2}});assert(check().ids.includes("2"),"old socket leave corrupted new snapshot");
 send(ws,"RECONNECT_READY",{fileGuid:"target",clients:[{id:1}]});assert(check().ids.includes("2"),"old socket initialization replaced new snapshot");
 ws.fire("close");assert(check().connected,"old socket close invalidated new connection");
 const native=new WebSocket();native.fire("message",JSON.stringify({type:"COLLABROOM",data:{type:"RECONNECT_READY",fileGuid:"target",clients:[{id:1},{id:3}]}}));assert(check().ready&&check().ids.includes("3"),"native object frame with clients array was corrupted");
 native.fire("message",JSON.stringify({type:"COLLABROOM",data:{type:"USER_LEAVE",fileGuid:"target",userInfo:{id:3}}}));assert(!check().ids.includes("3"),"native object leave not tracked");
 native.readyState=3;send(native,"RECONNECT_READY",{fileGuid:"target",clients:[]});assert(!check().connected,"closed socket initialization must not restore readiness");
 `
	cmd := exec.Command(node, "-e", setup+phase3PresenceObserver(`"target"`)+cases)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("observer regression: %v\n%s", err, out)
	}
}

func TestPhase3ReadinessPoll(t *testing.T) {
	calls := 0
	n, err := phase3PollReady(time.Second, time.Millisecond, func() (map[string]any, error) {
		calls++
		return map[string]any{"ready": calls == 3, "documentState": "loading"}, nil
	})
	if err != nil || n != 3 {
		t.Fatalf("delayed readiness: n=%d err=%v", n, err)
	}
	calls = 0
	n, err = phase3PollReady(time.Second, time.Millisecond, func() (map[string]any, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("Execution context was destroyed")
		}
		return map[string]any{"ready": true}, nil
	})
	if err != nil || n != 2 {
		t.Fatalf("navigation retry failed: %d %v", n, err)
	}
	calls = 0
	started := time.Now()
	n, err = phase3PollReady(3*time.Millisecond, time.Millisecond, func() (map[string]any, error) {
		calls++
		return map[string]any{"ready": false, "documentState": "complete"}, nil
	})
	// Timer granularity may allow only one probe before this short deadline.
	// The ready/navigation cases above separately require actual retries.
	if err == nil || n < 1 || n != calls || !strings.Contains(err.Error(), "超时") || time.Since(started) < 3*time.Millisecond {
		t.Fatalf("timeout not detected: %d %v", n, err)
	}
	n, err = phase3PollReady(time.Second, time.Millisecond, func() (map[string]any, error) { return nil, errors.New("actual method exception") })
	if err == nil || n != 1 {
		t.Fatalf("real exception retried: %d %v", n, err)
	}
}

func TestPhase3ReadinessRPCUsesRemainingBudget(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	received := make(chan []byte, 1)
	go func() {
		ws := &WSConn{conn: server, rd: bufio.NewReader(server)}
		message, _ := ws.readMessage()
		received <- message
	}()
	b := &phase3EditorBrowser{ws: &WSConn{conn: client, rd: bufio.NewReader(client)}}
	started := time.Now()
	err := b.WaitReady("false", 25*time.Millisecond)
	var failed *phase3ReadinessFailure
	if !errors.As(err, &failed) {
		t.Fatalf("read RPC timeout must not trigger constructor replay: %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("readiness RPC exceeded total deadline")
	}
	if len(<-received) == 0 || b.readyAttempts != 1 {
		t.Fatalf("unexpected retry after potentially partial frame: attempts=%d", b.readyAttempts)
	}
}

func TestPhase3DebugHandshakeDeadline(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		c, e := listener.Accept()
		if e != nil {
			return
		}
		defer c.Close()
		_, _ = readHTTPHead(bufio.NewReader(c))
		<-done
	}()
	start := time.Now()
	ws, err := dialWSWithTimeout("ws://"+listener.Addr().String()+"/devtools/page/test", 30*time.Millisecond)
	if ws != nil {
		ws.Close()
		t.Fatal("server never upgraded handshake")
	}
	if err == nil {
		t.Fatal("handshake unexpectedly succeeded")
	}
	if time.Since(start) > time.Second {
		t.Fatalf("HTTP handshake ignored deadline: %v", time.Since(start))
	}
}

func TestPhase3ReadinessRetainsLastRealState(t *testing.T) {
	calls := 0
	_, err := phase3PollReady(time.Second, time.Millisecond, func() (map[string]any, error) {
		calls++
		if calls == 1 {
			return map[string]any{"ready": false, "documentState": "complete", "presence": "pending"}, nil
		}
		return map[string]any{"budgetExpired": true}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "pending") || strings.Contains(err.Error(), "budgetExpired") {
		t.Fatalf("last real state lost: %v", err)
	}
}

type phase3DeadlineRecorder struct {
	net.Conn
	deadlines []time.Time
}

func (c *phase3DeadlineRecorder) SetDeadline(t time.Time) error {
	c.deadlines = append(c.deadlines, t)
	return c.Conn.SetDeadline(t)
}
func TestPhase3ReadOnlyBudgetDoesNotExtendMutation(t *testing.T) {
	for _, readOnly := range []bool{true, false} {
		client, server := net.Pipe()
		conn := &phase3DeadlineRecorder{Conn: client}
		b := &phase3EditorBrowser{ws: &WSConn{conn: conn, rd: bufio.NewReader(conn)}}
		go func() {
			defer server.Close()
			peer := &WSConn{conn: server, rd: bufio.NewReader(server)}
			raw, err := peer.readMessage()
			if err != nil {
				return
			}
			var request map[string]any
			_ = json.Unmarshal(raw, &request)
			_ = peer.sendJSON(map[string]any{"id": request["id"], "result": map[string]any{"result": map[string]any{"value": map[string]any{"ok": true}}}})
		}()
		started := time.Now()
		var err error
		if readOnly {
			_, err = b.EvalReadOnly("(()=>({ok:true}))()")
		} else {
			_, err = b.Eval("(()=>({ok:true}))()")
		}
		client.Close()
		if err != nil {
			t.Fatal(err)
		}
		want := 30 * time.Second
		if readOnly {
			want = 90 * time.Second
		}
		if len(conn.deadlines) < 1 || conn.deadlines[0].Sub(started) < want-time.Second || conn.deadlines[0].Sub(started) > want+time.Second {
			t.Fatalf("unexpected budget readOnly=%v: %v", readOnly, conn.deadlines)
		}
	}
}
func TestPhase3CaptureFailureHasStageAndReadOnlyBudget(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	conn := &phase3DeadlineRecorder{Conn: client}
	b := &phase3EditorBrowser{ws: &WSConn{conn: conn, rd: bufio.NewReader(conn)}}
	go func() {
		defer server.Close()
		peer := &WSConn{conn: server, rd: bufio.NewReader(server)}
		raw, err := peer.readMessage()
		if err != nil {
			return
		}
		var request map[string]any
		_ = json.Unmarshal(raw, &request)
		_ = peer.sendJSON(map[string]any{"id": request["id"], "error": map[string]any{"message": "test screenshot timeout"}})
	}()
	started := time.Now()
	err := b.Capture(filepath.Join(t.TempDir(), "screenshot.png"))
	if err == nil || !strings.Contains(err.Error(), "Page.captureScreenshot") {
		t.Fatalf("missing screenshot stage: %v", err)
	}
	if len(conn.deadlines) < 1 || conn.deadlines[0].Sub(started) < 89*time.Second || conn.deadlines[0].Sub(started) > 91*time.Second {
		t.Fatal("screenshot did not get bounded 90-second read-only budget")
	}
}

func TestPhase3UserGestureKeepsSingleMutationBudget(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	conn := &phase3DeadlineRecorder{Conn: client}
	b := &phase3EditorBrowser{ws: &WSConn{conn: conn, rd: bufio.NewReader(conn)}}
	requests := make(chan map[string]any, 1)
	go func() {
		defer server.Close()
		peer := &WSConn{conn: server, rd: bufio.NewReader(server)}
		raw, err := peer.readMessage()
		if err != nil {
			return
		}
		var request map[string]any
		_ = json.Unmarshal(raw, &request)
		requests <- request
		_ = peer.sendJSON(map[string]any{"id": request["id"], "result": map[string]any{"result": map[string]any{"value": map[string]any{"ok": true}}}})
	}()
	started := time.Now()
	if _, err := b.EvalUserGesture("playNativePlayerOnce()"); err != nil {
		t.Fatal(err)
	}
	request := <-requests
	params, _ := request["params"].(map[string]any)
	if request["method"] != "Runtime.evaluate" || params["userGesture"] != true || params["expression"] != "playNativePlayerOnce()" {
		t.Fatal(request)
	}
	if len(conn.deadlines) == 0 || conn.deadlines[0].Sub(started) < 29*time.Second || conn.deadlines[0].Sub(started) > 31*time.Second {
		t.Fatal(conn.deadlines)
	}
}

func TestPhase3PresenceInstallationEnablesDomainBeforeScript(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	b := &phase3EditorBrowser{ws: &WSConn{conn: client, rd: bufio.NewReader(client)}, presenceObserver: "fixed observer"}
	requests := make(chan map[string]any, 2)
	go func() {
		defer server.Close()
		peer := &WSConn{conn: server, rd: bufio.NewReader(server)}
		for i := 0; i < 2; i++ {
			raw, e := peer.readMessage()
			if e != nil {
				return
			}
			var request map[string]any
			_ = json.Unmarshal(raw, &request)
			requests <- request
			result := map[string]any{}
			if i == 1 {
				result["identifier"] = "real-script-id"
			}
			_ = peer.sendJSON(map[string]any{"id": request["id"], "result": result})
		}
	}()
	if err := b.installPresenceObserver(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	first, second := <-requests, <-requests
	if first["method"] != "Page.enable" || second["method"] != "Page.addScriptToEvaluateOnNewDocument" {
		t.Fatal(first, second)
	}
	params, _ := second["params"].(map[string]any)
	if params["source"] != "fixed observer" {
		t.Fatal(params)
	}
}

func TestPhase3ReadinessRetainsLastStateOnTransportFailure(t *testing.T) {
	calls := 0
	_, err := phase3PollReady(time.Second, time.Millisecond, func() (map[string]any, error) {
		calls++
		if calls == 1 {
			return map[string]any{"ready": false, "sdk": "undefined", "visibility": "hidden", "hasFocus": false, "canvasCount": 0}, nil
		}
		return nil, errors.New("read tcp: i/o timeout")
	})
	if calls != 2 || err == nil || !strings.Contains(err.Error(), `"visibility":"hidden"`) || !strings.Contains(err.Error(), "i/o timeout") {
		t.Fatalf("lost pre-stall observation: calls=%d err=%v", calls, err)
	}
}

func TestPhase3InitialNavigationFocusOrderAndFailure(t *testing.T) {
	for _, refuseFocus := range []bool{false, true} {
		client, server := net.Pipe()
		conn := &phase3DeadlineRecorder{Conn: client}
		b := &phase3EditorBrowser{ws: &WSConn{conn: conn, rd: bufio.NewReader(conn)}}
		requests := make(chan map[string]any, 2)
		go func() {
			defer server.Close()
			peer := &WSConn{conn: server, rd: bufio.NewReader(server)}
			for i := 0; i < 2; i++ {
				raw, e := peer.readMessage()
				if e != nil {
					return
				}
				var request map[string]any
				_ = json.Unmarshal(raw, &request)
				requests <- request
				response := map[string]any{"id": request["id"], "result": map[string]any{}}
				if refuseFocus {
					response = map[string]any{"id": request["id"], "error": map[string]any{"message": "i/o timeout"}}
				}
				_ = peer.sendJSON(response)
				if refuseFocus {
					return
				}
			}
		}()
		deadline := time.Now().Add(time.Second)
		err := b.navigateInitial("https://shimo.net/docx/fixture", deadline)
		client.Close()
		first := <-requests
		if first["method"] != "Page.bringToFront" {
			t.Fatal(first)
		}
		if refuseFocus {
			var failure *phase3ReadinessFailure
			if err == nil || !errors.As(err, &failure) || !strings.Contains(err.Error(), "Page.bringToFront") || !strings.Contains(err.Error(), "本次仅执行只读导航") || strings.Contains(err.Error(), "未执行内容写入") || b.nextID != 1 {
				t.Fatalf("focus failure retried/navigated: %v, calls=%d", err, b.nextID)
			}
		} else {
			if err != nil {
				t.Fatal(err)
			}
			second := <-requests
			params, _ := second["params"].(map[string]any)
			if second["method"] != "Page.navigate" || params["url"] != "https://shimo.net/docx/fixture" {
				t.Fatal(second)
			}
		}
		for _, d := range conn.deadlines {
			if !d.IsZero() && d.After(deadline.Add(time.Millisecond)) {
				t.Fatal("navigation extended shared budget")
			}
		}
	}
}

func TestPhase3InitialNavigationExpiredBudgetDoesNotSend(t *testing.T) {
	b := &phase3EditorBrowser{}
	if err := b.navigateInitial("https://shimo.net/docx/fixture", time.Now().Add(-time.Second)); err == nil || strings.Contains(err.Error(), "未执行内容写入") || !strings.Contains(err.Error(), "本次仅执行只读导航") || b.nextID != 0 {
		t.Fatalf("expired initial budget sent request: %v", err)
	}
}

func TestPhase3PresenceGuardErrorsDoNotClaimOverallWriteState(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("existing Node for browser guard execution")
	}
	script := `global.window={};const check=()=>{` + phase3PresenceGuardJS() + `return {ok:true}};for(const state of [null,{ready:true,connected:true,ids:['another']}]){window.__shimoPhase3Presence=()=>state;window._sdk={options:{user:{id:'self'}}};const result=check();if(result.ok!==false||!result.error.includes('当前协作名单检查未通过')||/未执行写入|已中止写入/.test(result.error))throw Error('shared readback guard claims whole-command write state: '+result.error)}`
	if out, e := exec.Command(node, "-e", script).CombinedOutput(); e != nil {
		t.Fatalf("%v %s", e, out)
	}
}

// The fast-fail rule must never fire on a page that is already collaborating:
// a signed-in table can take longer than the grace period to inject _sdk.
func TestPhase3PresenceReadyGate(t *testing.T) {
	cases := []struct {
		state map[string]any
		want  bool
	}{
		{map[string]any{"presence": map[string]any{"ready": true, "connected": true}}, true},
		{map[string]any{"presence": map[string]any{"ready": true, "connected": false}}, false},
		{map[string]any{"presence": map[string]any{"ready": false, "connected": true}}, false},
		{map[string]any{"presence": nil}, false},
		{map[string]any{}, false},
	}
	for i, c := range cases {
		if got := phase3PresenceReady(c.state); got != c.want {
			t.Fatalf("case %d: got %v want %v", i, got, c.want)
		}
	}
}
