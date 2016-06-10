package main

import (
    "github.com/jinzhu/gorm"
)

const (
    GuestUser   = iota
    RegularUser
    AdminUser
)

type User struct {
    gorm.Model
    Type  int64  `json:"type"`
    Email string `json:"email"`
}
