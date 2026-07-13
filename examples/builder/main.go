package main

import (
	"github.com/Angus-Warman/httpmin"
	"github.com/Angus-Warman/httpmin/handler"
)

type User struct {
	ID     string
	Name   string
	Active bool
}

type UserStore struct {
	users []User
}

func (u UserStore) Add(user User) (User, error) {
	panic("unimplemented")
}

// Delete implements [handler.Store].
func (u UserStore) Delete(user User) (bool, error) {
	panic("unimplemented")
}

// Get implements [handler.Store].
func (u UserStore) Get(string) (User, error) {
	panic("unimplemented")
}

// GetAll implements [handler.Store].
func (u UserStore) GetAll() ([]User, error) {
	panic("unimplemented")
}

// Update implements [handler.Store].
func (u UserStore) Update(user User) (User, error) {
	panic("unimplemented")
}

func main() {
	c := httpmin.New()

	store := UserStore{}

	handler.Builder(c.Mux, "/api/users", store)
}
