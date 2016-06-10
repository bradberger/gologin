package main

import (
    "github.com/jinzhu/gorm"
)

type Data struct {
    gorm.Model
    Type  int64   `json:"type"`
    Key   string  `json:"key"`
    Value string  `json:"value"`
}
