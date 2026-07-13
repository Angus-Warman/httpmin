package main

import (
	"fmt"
	"sync"
)

type User struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

type UserStore struct {
	mu    sync.RWMutex
	users []User
	next  int
}

func (u *UserStore) GetAll() ([]User, error) {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.users, nil
}

func (u *UserStore) Get(id string) (User, error) {
	u.mu.RLock()
	defer u.mu.RUnlock()

	for _, user := range u.users {
		if user.ID == id {
			return user, nil
		}
	}

	return User{}, fmt.Errorf("user %s not found", id)
}

func (u *UserStore) Add(user User) (User, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.next++
	user.ID = fmt.Sprintf("%d", u.next)
	u.users = append(u.users, user)
	return user, nil
}

func (u *UserStore) Update(user User) (User, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	for i, existing := range u.users {
		if existing.ID == user.ID {
			u.users[i] = user
			return user, nil
		}
	}

	return User{}, fmt.Errorf("user %s not found", user.ID)
}

func (u *UserStore) Delete(id string) (bool, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	for i, user := range u.users {
		if user.ID == id {
			u.users = append(u.users[:i], u.users[i+1:]...)
			return true, nil
		}
	}

	return false, nil
}
