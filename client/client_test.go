package client

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mtgo-labs/mtgo/telegram"
	"github.com/mtgo-labs/mtgo/tg"
)

func TestEncodeTLToJSON(t *testing.T) {
	data, err := encodeTLToJSON(&tg.UpdateNewMessage{
		Message:  &tg.MessageEmpty{ID: 42},
		PTS:      123,
		PTSCount: 1,
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	want := `{"_":"updateNewMessage","message":{"id":42},"pts":123,"pts_count":1}`
	if string(data) != want {
		t.Fatalf("got %s, want %s", data, want)
	}
}

// A raw update is dropped silently when encoding fails, so a schema bump that
// adds a type this encoder can't handle would go unnoticed until that update
// arrives. Run it over every constructor in mtgo's generated schema.
func TestEncodeTLToJSONCoversSchema(t *testing.T) {
	idToName := getIDToName()
	if len(tg.ConstructorMap) == 0 {
		t.Fatal("tg.ConstructorMap is empty")
	}

	for id, factory := range tg.ConstructorMap {
		name := idToName[id]
		data, err := encodeTLToJSON(factory())
		if err != nil {
			t.Fatalf("%s: encode: %v", name, err)
		}
		var v map[string]any
		if err := json.Unmarshal(data, &v); err != nil {
			t.Fatalf("%s: not a JSON object: %v (%s)", name, err, data)
		}
		if v["_"] != name {
			t.Fatalf("%s: _ = %v", name, v["_"])
		}
	}
}

func TestHandleRawUpdate(t *testing.T) {
	c := New()
	var got []json.RawMessage
	c.OnRawUpdate(func(data json.RawMessage) { got = append(got, data) })

	c.handleRawUpdate(&telegram.Context{Update: &telegram.Update{Raw: &tg.UpdateUserTyping{UserID: 42}}})
	if len(got) != 1 {
		t.Fatalf("forwarded %d updates, want 1", len(got))
	}
	want := `{"_":"updateUserTyping","user_id":42}`
	if string(got[0]) != want {
		t.Fatalf("got %s, want %s", got[0], want)
	}

	// No raw update — nothing to forward.
	c.handleRawUpdate(&telegram.Context{})
	c.handleRawUpdate(&telegram.Context{Update: &telegram.Update{}})
	if len(got) != 1 {
		t.Fatalf("forwarded %d updates without a raw update, want 1", len(got))
	}
}

func TestHandleRawUpdateNoHandler(t *testing.T) {
	// No handler registered — should not panic.
	New().handleRawUpdate(&telegram.Context{Update: &telegram.Update{Raw: &tg.UpdateUserTyping{}}})
}

// mtgo decides between catch-all and type-filtered raw-update handling from
// the callback's type, so the registered handler must land in mtgo's catch-all
// mode and forward every update type through mtgo's own dispatcher.
func TestRawUpdateHandlerIsCatchAll(t *testing.T) {
	c := New()
	var got []json.RawMessage
	c.OnRawUpdate(func(data json.RawMessage) { got = append(got, data) })

	h := telegram.NewRawUpdateHandler(c.handleRawUpdate)
	for _, raw := range []tg.TLObject{
		&tg.UpdateUserTyping{UserID: 42},
		&tg.UpdateNewMessage{Message: &tg.MessageEmpty{ID: 7}},
	} {
		upd := &telegram.Update{Raw: raw}
		if !h.Check(upd) {
			t.Fatalf("%T: handler does not match this update type", raw)
		}
		h.Handle(&telegram.Context{Update: upd})
	}
	if len(got) != 2 {
		t.Fatalf("forwarded %d updates, want 2", len(got))
	}
	want := `{"_":"updateNewMessage","message":{"id":7}}`
	if string(got[1]) != want {
		t.Fatalf("got %s, want %s", got[1], want)
	}
}

// Every IPC method must answer sensibly before mtgo.connect, without
// touching the network.
func TestClientBeforeConnect(t *testing.T) {
	c := New()

	if c.Connected() {
		t.Fatal("Connected() = true before Connect")
	}
	if _, err := c.Invoke(context.Background(), "help.getConfig", nil); err == nil {
		t.Fatal("Invoke before Connect: expected an error")
	}

	info, err := c.Info()
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.(map[string]any)["connected"] != false {
		t.Fatalf("Info: connected = %v, want false", info.(map[string]any)["connected"])
	}

	status, err := c.AuthStatus()
	if err != nil {
		t.Fatalf("AuthStatus: %v", err)
	}
	if status.(map[string]any)["authorized"] != false {
		t.Fatalf("AuthStatus: authorized = %v, want false", status.(map[string]any)["authorized"])
	}

	if err := c.Close(); err != nil {
		t.Fatalf("Close before Connect: %v", err)
	}
}

func TestConnectRequiresCredentials(t *testing.T) {
	c := New()
	if err := c.Connect(context.Background(), Config{}); err == nil {
		t.Fatal("Connect without credentials: expected an error")
	}
	if c.tg != nil {
		t.Fatal("Connect without credentials created an mtgo client")
	}
}

// telegram.invoke has no deadline of its own, so mtgo's ReqTimeout is the
// response window for every call. mtgo v0.19+ defaults it to 15s.
func TestMtgoConfig(t *testing.T) {
	mc, err := telegram.NewClient(12345, "api-hash", mtgoConfig(Config{
		Session:  "session-string",
		BotToken: "bot-token",
	}))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	cfg := mc.Config()
	if cfg.ReqTimeout != reqTimeout {
		t.Fatalf("ReqTimeout = %v, want %v", cfg.ReqTimeout, reqTimeout)
	}
	if cfg.SessionString != "session-string" || cfg.BotToken != "bot-token" {
		t.Fatalf("credentials not mapped: session=%q bot_token=%q", cfg.SessionString, cfg.BotToken)
	}
	if !cfg.InMemory {
		t.Fatal("InMemory = false, want true")
	}
}
