package rpc_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"

	overseasmapv1 "github.com/soneda-yuya/overseas-safety-map/gen/go/v1"
	"github.com/soneda-yuya/overseas-safety-map/gen/go/v1/overseasmapv1connect"
	"github.com/soneda-yuya/overseas-safety-map/internal/interfaces/rpc"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/authctx"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

// fakeVerifier accepts specific tokens and returns a deterministic uid;
// everything else fails with KindUnauthorized.
type fakeVerifier struct {
	accept map[string]string // token → uid
	fail   error
}

func (f *fakeVerifier) Verify(_ context.Context, tok string) (string, error) {
	if f.fail != nil {
		return "", f.fail
	}
	uid, ok := f.accept[tok]
	if !ok {
		return "", errs.Wrap("fake.verify", errs.KindUnauthorized, errors.New("bad token"))
	}
	return uid, nil
}

// echoUserService is a minimal UserProfileServiceHandler used only to assert
// that AuthInterceptor hands the correct uid to the handler. GetProfile is
// wired to read uid from ctx and return it on the response.
type echoUserService struct {
	overseasmapv1connect.UnimplementedUserProfileServiceHandler
	gotUID string
}

func (e *echoUserService) GetProfile(
	ctx context.Context,
	_ *connect.Request[overseasmapv1.GetProfileRequest],
) (*connect.Response[overseasmapv1.GetProfileResponse], error) {
	uid, _ := authctx.UIDFrom(ctx)
	e.gotUID = uid
	return connect.NewResponse(&overseasmapv1.GetProfileResponse{
		Profile: &overseasmapv1.UserProfile{Uid: uid},
	}), nil
}

// newTestServer wires the interceptor chain (Auth → Error) onto a stub
// UserProfileService and returns an HTTP test server + a generated client.
func newTestServer(t *testing.T, verifier *fakeVerifier) (*httptest.Server, overseasmapv1connect.UserProfileServiceClient, *echoUserService) {
	t.Helper()
	svc := &echoUserService{}
	mux := http.NewServeMux()
	path, handler := overseasmapv1connect.NewUserProfileServiceHandler(svc, connect.WithInterceptors(
		rpc.NewErrorInterceptor("dev"),
		rpc.NewAuthInterceptor(verifier, slog.Default()),
	))
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := overseasmapv1connect.NewUserProfileServiceClient(srv.Client(), srv.URL)
	return srv, client, svc
}

func TestAuth_ValidTokenPassesUIDToHandler(t *testing.T) {
	t.Parallel()
	_, client, svc := newTestServer(t, &fakeVerifier{accept: map[string]string{"good": "uid-xyz"}})
	req := connect.NewRequest(&overseasmapv1.GetProfileRequest{})
	req.Header().Set("Authorization", "Bearer good")
	if _, err := client.GetProfile(context.Background(), req); err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if svc.gotUID != "uid-xyz" {
		t.Errorf("handler saw uid = %q; want uid-xyz", svc.gotUID)
	}
}

func TestAuth_MissingHeaderIsUnauthenticated(t *testing.T) {
	t.Parallel()
	_, client, _ := newTestServer(t, &fakeVerifier{})
	_, err := client.GetProfile(context.Background(), connect.NewRequest(&overseasmapv1.GetProfileRequest{}))
	if err == nil {
		t.Fatal("expected error")
	}
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("code = %v; want Unauthenticated", connect.CodeOf(err))
	}
}

func TestAuth_InvalidTokenIsUnauthenticated(t *testing.T) {
	t.Parallel()
	_, client, _ := newTestServer(t, &fakeVerifier{accept: map[string]string{"good": "uid"}})
	req := connect.NewRequest(&overseasmapv1.GetProfileRequest{})
	req.Header().Set("Authorization", "Bearer bad")
	_, err := client.GetProfile(context.Background(), req)
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("code = %v; want Unauthenticated", connect.CodeOf(err))
	}
}

func TestAuth_NonBearerSchemeIsUnauthenticated(t *testing.T) {
	t.Parallel()
	_, client, _ := newTestServer(t, &fakeVerifier{})
	req := connect.NewRequest(&overseasmapv1.GetProfileRequest{})
	req.Header().Set("Authorization", "Basic something")
	_, err := client.GetProfile(context.Background(), req)
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("code = %v; want Unauthenticated", connect.CodeOf(err))
	}
}
