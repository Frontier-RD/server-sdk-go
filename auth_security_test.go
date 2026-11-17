package lksdk_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
)

func TestSDKScopedAdminAuth(t *testing.T) {
	const key = "b03-local-fixture-key"
	const secret = "b03-local-fixture-secret-at-least-32-bytes"
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.Path != "/twirp/livekit.RoomService/ListRooms" {
			t.Errorf("unexpected RPC %s", r.URL.Path)
		}
		v, err := auth.ParseAPIToken(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if err != nil {
			t.Error(err)
			w.WriteHeader(401)
			return
		}
		_, grants, err := v.Verify(secret)
		if err != nil {
			t.Error(err)
			w.WriteHeader(401)
			return
		}
		if v.APIKey() != key || grants.Video == nil || !grants.Video.RoomList || grants.Video.RoomAdmin || grants.Video.RoomJoin {
			t.Error("wrong SDK room-list grants")
		}
		if _, _, err := v.Verify("incorrect-fixture-secret"); err == nil {
			t.Error("wrong HMAC secret accepted")
		}
		w.Header().Set("Content-Type", "application/protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	if _, err := lksdk.NewRoomServiceClient(server.URL, key, secret).ListRooms(context.Background(), &livekit.ListRoomsRequest{}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("SDK did not call transport")
	}
	token, err := auth.NewAccessToken(key, secret).SetIdentity("b03-fixture").SetValidFor(time.Minute).SetVideoGrant(&auth.VideoGrant{RoomJoin: true, Room: "b03-local-room"}).ToJWT()
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := auth.ParseAPIToken(token)
	if err != nil {
		t.Fatal(err)
	}
	_, grants, err := verifier.Verify(secret)
	if err != nil {
		t.Fatal(err)
	}
	if grants.Identity != "b03-fixture" || grants.Video.Room != "b03-local-room" || !grants.Video.RoomJoin {
		t.Fatal("join grants changed")
	}
}

func TestBenignMalformedKeyRejected(t *testing.T) {
	for _, input := range []string{`{"kty":"RSA","n":"*","e":"AQAB"}`, `{"kty":"RSA","e":"AQAB"}`, `{"kty":"EC","crv":"unknown","x":"AQ","y":"AQ"}`} {
		var key jose.JSONWebKey
		if err := json.Unmarshal([]byte(input), &key); err == nil {
			t.Errorf("malformed JWK accepted: %s", input)
		}
	}
	for _, input := range []string{"", "not-a-token", "e30.e30.invalid"} {
		v, err := auth.ParseAPIToken(input)
		if err == nil {
			_, _, err = v.Verify("b03-local-fixture-secret-at-least-32-bytes")
		}
		if err == nil {
			t.Errorf("malformed token accepted: %s", input)
		}
	}
}
