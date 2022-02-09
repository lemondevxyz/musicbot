package main

type Config struct {
	BotToken   string `envconfig:"BOT_TOKEN"`
	YoutubeKey string `envconfig:"YOUTUBE_KEY"`
	Prefix     string `envconfig:"PREFIX"`
	Status     string `envconfig:"STATUS"`
}

var config Config
