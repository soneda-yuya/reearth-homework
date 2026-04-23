package application_test

import (
	"context"
	"errors"
	"sync"

	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
	"github.com/soneda-yuya/overseas-safety-map/internal/user/domain"
)

// fakeProfileRepo is an in-memory ProfileRepository used across the user/
// application tests. Concurrency protection is overkill for the current
// tests but keeps the fake safe if a future test runs Execute in goroutines.
type fakeProfileRepo struct {
	mu       sync.Mutex
	store    map[string]domain.UserProfile
	getErr   error
	createErr error
	toggleErr error
	updateErr error
	registerErr error
}

func newFakeProfileRepo() *fakeProfileRepo {
	return &fakeProfileRepo{store: make(map[string]domain.UserProfile)}
}

func (f *fakeProfileRepo) Get(_ context.Context, uid string) (*domain.UserProfile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return nil, f.getErr
	}
	profile, ok := f.store[uid]
	if !ok {
		return nil, errs.Wrap("fake.get", errs.KindNotFound, errors.New("no such uid"))
	}
	cp := profile
	return &cp, nil
}

func (f *fakeProfileRepo) CreateIfMissing(_ context.Context, profile domain.UserProfile) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return f.createErr
	}
	if _, exists := f.store[profile.UID]; exists {
		return nil
	}
	f.store[profile.UID] = profile
	return nil
}

func (f *fakeProfileRepo) ToggleFavoriteCountry(_ context.Context, uid, countryCd string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.toggleErr != nil {
		return f.toggleErr
	}
	profile, ok := f.store[uid]
	if !ok {
		return errs.Wrap("fake.toggle", errs.KindNotFound, errors.New("no such uid"))
	}
	for i, cc := range profile.FavoriteCountryCds {
		if cc == countryCd {
			profile.FavoriteCountryCds = append(profile.FavoriteCountryCds[:i], profile.FavoriteCountryCds[i+1:]...)
			f.store[uid] = profile
			return nil
		}
	}
	profile.FavoriteCountryCds = append(profile.FavoriteCountryCds, countryCd)
	f.store[uid] = profile
	return nil
}

func (f *fakeProfileRepo) UpdateNotificationPreference(_ context.Context, uid string, pref domain.NotificationPreference) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.updateErr != nil {
		return f.updateErr
	}
	profile, ok := f.store[uid]
	if !ok {
		return errs.Wrap("fake.update", errs.KindNotFound, errors.New("no such uid"))
	}
	profile.NotificationPreference = pref
	f.store[uid] = profile
	return nil
}

func (f *fakeProfileRepo) RegisterFcmToken(_ context.Context, uid, token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.registerErr != nil {
		return f.registerErr
	}
	profile, ok := f.store[uid]
	if !ok {
		return errs.Wrap("fake.register", errs.KindNotFound, errors.New("no such uid"))
	}
	for _, t := range profile.FCMTokens {
		if t == token {
			return nil
		}
	}
	profile.FCMTokens = append(profile.FCMTokens, token)
	f.store[uid] = profile
	return nil
}
