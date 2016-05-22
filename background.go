package main

import (
	"log"
	"net/http"
	"strconv"
	"time"
)

func backgroundConfigCheck() {
	var enableBackground config
	db.Get(&enableBackground, `select value from config where key = 'background-check'`)

	if enableBackground.Value != "" {
		b := background{}
		if err := b.Start(enableBackground.Value); err != nil {
			log.Println(err)
		}
	}
}

var backgroundCheck *time.Ticker

type backgroundWriter struct {
	TokenID int64
}

func (backgroundWriter) Header() http.Header {
	return make(http.Header)
}
func (w backgroundWriter) Write(data []byte) (int, error) {
	log.Printf("%d: %s\n", w.TokenID, string(data))
	return -1, nil
}
func (backgroundWriter) WriteHeader(int) {}
func (backgroundWriter) Flush()          {}

type background struct{}

func (b background) Start(duration string) error {
	hours, err := strconv.Atoi(duration)
	if err != nil {
		return err
	}

	if backgroundCheck != nil {
		backgroundCheck.Stop()
	}

	t := time.Hour * time.Duration(hours)

	log.Printf("Background update enabled for every %f hours.", t.Hours())

	backgroundCheck = time.NewTicker(t)

	time.Sleep(time.Second * 10)

	b.check()
	go func() {
		for range backgroundCheck.C {
			b.check()
		}
	}()

	return nil
}

func (background) Stop() {
	if backgroundCheck != nil {
		backgroundCheck.Stop()
	}
}

func (background) check() {
	log.Println("Running background check.")

	var tokens []token
	db.Select(&tokens, `select * from tokens`)

	for _, token := range tokens {
		writer := backgroundWriter{token.ID}
		load(token.ID, writer)
	}
}
