package application_test

import (
	"context"
	"errors"
	"sync"

	"github.com/soneda-yuya/reearth-homework/internal/notification/domain"
	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

type fakeDedup struct {
	mu   sync.Mutex
	seen map[string]bool
	err  error // force Dedup.CheckAndMark error
}

func newFakeDedup() *fakeDedup {
	return &fakeDedup{seen: map[string]bool{}}
}

func (f *fakeDedup) CheckAndMark(_ context.Context, keyCd string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.seen[keyCd] {
		return true, nil
	}
	f.seen[keyCd] = true
	return false, nil
}

type fakeUserRepo struct {
	mu           sync.Mutex
	subscribers  map[string][]domain.UserProfile // key: countryCd
	removed      map[string][]string             // uid → tokens removed
	findErr      error
	removeErrFor string // uid → force error
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{
		subscribers: map[string][]domain.UserProfile{},
		removed:     map[string][]string{},
	}
}

func (f *fakeUserRepo) FindSubscribers(_ context.Context, countryCd, _ string) ([]domain.UserProfile, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]domain.UserProfile{}, f.subscribers[countryCd]...), nil
}

func (f *fakeUserRepo) RemoveInvalidTokens(_ context.Context, uid string, tokens []string) error {
	if uid == f.removeErrFor {
		return errors.New("remove failed")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed[uid] = append(f.removed[uid], tokens...)
	return nil
}

type fakeFCM struct {
	mu       sync.Mutex
	calls    []domain.FCMMessage
	resultFn func(msg domain.FCMMessage) (domain.BatchResult, error)
}

func (f *fakeFCM) SendMulticast(_ context.Context, msg domain.FCMMessage) (domain.BatchResult, error) {
	f.mu.Lock()
	f.calls = append(f.calls, msg)
	f.mu.Unlock()
	if f.resultFn != nil {
		return f.resultFn(msg)
	}
	return domain.BatchResult{SuccessCount: len(msg.Tokens)}, nil
}

func transient(msg string) error {
	return errs.Wrap("fake", errs.KindExternal, errors.New(msg))
}
