package main

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/rylio/ytdl"
	"github.com/spf13/viper"
	"google.golang.org/api/googleapi/transport"
	"google.golang.org/api/youtube/v3"
	"layeh.com/gopus"
)

type commandCallback func(s *discordgo.Session, m *commandParameter)
type commandParameter struct {
	*discordgo.MessageCreate
	Split []string
}

type videoInfo struct {
	Base *ytdl.VideoInfo
	Name string
}

var sesh *discordgo.Session
var commands = map[string]commandCallback{
	"play":       cmdPlay,  // Add a song to the queue
	"ping":       cmdPing,  // Send a ping back at the user. used to check if message handler is working
	"queue":      cmdQueue, // Lists out the current queue
	"skip":       cmdSkip,  // Skips the current song
	"loop":       cmdLoop,  // Set the cmdLoop( to off, current song, or current queue
	"join":       cmdJoin,
	"volume":     cmdVolume,
	"pause":      cmdPause,
	"resume":     cmdResume,
	"playsample": cmdPlaySample,
	"setname":    cmdSetName,
	"setavatar":  cmdSetAvatar,
	"shuffle":    cmdShuffle,
	"clear":      cmdClear,
	//"speed":      cmdSpeed,
}

var yt *youtube.Service
var vc *discordgo.VoiceConnection

var queue = []*videoInfo{}
var queueindex = -1 // This is the original queue index
var listeners []chan int

var loop = loopOff
var shuffle = false

var input *os.File
var output *os.File

var sample = []string{
	"https://www.youtube.com/watch?v=eCGV26aj-mM",
	"https://www.youtube.com/watch?v=XbGs_qK2PQA",
	"https://www.youtube.com/watch?v=MwUxHH1G6Do",
	"https://www.youtube.com/watch?v=tvTRZJ-4EyI",
	"https://www.youtube.com/watch?v=xk9EuEwMKcM",
}

const (
	loopOff   = iota // No cmdLoop(
	loopSong         // cmdLoop( current song
	loopQueue        // cmdLoop( current queue
)

var pause = false

func main() {

	go func() {
		<-sendchannel
	}()

	viper.SetConfigName("config")
	viper.AddConfigPath(".")
	viper.SetDefault("botToken", "")
	viper.SetDefault("youtubeKey", "")
	viper.SetDefault("prefix", "")

	var err error

	err = viper.ReadInConfig()
	if err != nil {
		log.Fatalf("Cannot read config, error: %v", err)
	}

	err = viper.Unmarshal(&config)
	if err != nil {
		log.Fatalf("Unable to unmarshal config, error: %v", err)
	}

	log.Fatalln(config)

	input, err = os.Create(audioFilename)
	if err != nil {
		log.Fatalf("Cannot create input file, error: %v", err)
	}

	output, err = os.Create(dcaFilename)
	if err != nil {
		log.Fatalf("Cannot create output file, error: %v", err)
	}

	cleanup()
	defer cleanup()

	opusEncoder, err = gopus.NewEncoder(audioFrameRate, audioChannels, gopus.Audio)
	if err != nil {
		log.Fatalf("An error occured with initializing the OpusEncoder, error: %v", err)
	}

	opusEncoder.SetBitrate(audioBitrate * 1000)
	opusEncoder.SetApplication(gopus.Audio)

	ytdlclient := ytdl.DefaultClient

	go func() {

		queuechannel := getchan()

		for {
			<-queuechannel
			if pause {
				pause = false
			}

			if queueindex < len(queue) {
				vid := queue[queueindex]

				ytdlclient.Download(vid.Base, vid.Base.Formats[0], input)
				time.Sleep(time.Millisecond * 1000)

				send()
				cleanup()
			}

			time.Sleep(time.Millisecond * 250)
		}

	}()

	// Create a new session, this initializes the session to add handlers
	sesh, err = discordgo.New(fmt.Sprintf("Bot %s", config.BotToken))
	if err != nil {
		log.Fatalf("An error occured with creating a new session, error: %v", err)
	}

	// Add a message handler to sort commands from regular messages
	sesh.AddHandler(messageHandler)

	// Open a websocket connection, to make the bot online and useable to users.
	err = sesh.Open()
	if err != nil {
		log.Fatalf("An error occured with opening the websocket connecting, error: %v", err)
	}

	client := &http.Client{
		Transport: &transport.APIKey{Key: config.YoutubeKey},
	}

	yt, err = youtube.New(client)
	if err != nil {
		log.Fatalf("Error creating new YouTube client: %v", err)
	}

	log.Printf("BOT TOKEN: '%v'", config.BotToken)
	log.Println("Session created successfully")
	sig := make(chan os.Signal)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM, os.Kill, syscall.SIGINT)
	<-sig

	// Close the session
	sesh.Close()
	log.Println("Closed Session")
}

func cleanup() {

	input.Truncate(0)
	input.Seek(0, 0)

	output.Truncate(0)
	output.Seek(0, 0)

}

func cleanlistener(listener chan int) {
	var index = -1
	for k, v := range listeners {
		if v == listener {
			index = k
			break
		}
	}

	if index >= 0 {
		listeners[len(listeners)-1], listeners[index] = listeners[index], listeners[len(listeners)-1]
		listeners = listeners[:len(listeners)-1]
	}

}

func setqueueindex(v int) {
	if len(queue) > v {
		queueindex = v
		for _, ch := range listeners {
			ch <- v
		}
	}
}

func getchan() chan int {
	listener := make(chan int, 100)

	listeners = append(listeners, listener)
	return listener
}

func messageHandler(s *discordgo.Session, m *discordgo.MessageCreate) {
	// If the author of the message is the same as the bot
	// i.e if the bot sent the message
	if m.Author.ID == s.State.User.ID {
		return
	}

	if !strings.HasPrefix(m.Content, config.Prefix) {
		return
	}

	m.Content = strings.TrimPrefix(m.Content, config.Prefix)
	split := strings.Split(m.Content, " ")

	if len(split) > 0 {
		cp := &commandParameter{
			m,
			split,
		}

		if fn, ok := commands[split[0]]; ok {
			fn(s, cp)
		}
	}

}

func cmdPlay(s *discordgo.Session, m *commandParameter) {
	if len(m.Split) >= 2 {
		urlstring := m.Split[1]
		uri, err := url.ParseRequestURI(urlstring)
		yturl := ""
		if err == nil {
			yturl = uri.String()
		} else {
			for k, v := range m.Split[1:] {
				if k+1 != len(m.Split) {
					urlstring += v + " "
				}
			}

			call := yt.Search.List("id").Q(urlstring).MaxResults(1).Type("video")

			res, err := call.Do()
			if err != nil {
				s.ChannelMessageSend(m.ChannelID, "An error error occured with searching for the video, please consult @toms#1441")
			}

			if len(res.Items) > 0 {
				yturl = "https://youtube.com/watch?v=" + res.Items[0].Id.VideoId
			}
		}

		if len(yturl) > 0 {
			vid, err := ytdl.GetVideoInfo(yturl)
			if err == nil {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Added **%s** to the queue!", vid.Title))
				queue = append(queue, &videoInfo{
					Base: vid,
					Name: "@" + m.Author.String(),
				})

				if queueindex == -1 {
					setqueueindex(0)
				}

				if vc == nil {
					cmdJoin(s, m)
				}
			}
		} else {
			s.ChannelMessageSend(m.ChannelID, "No videos found!")
		}
	} else {
		if pause {
			cmdResume(s, m)
		} else {
			s.ChannelMessageSend(m.ChannelID, "Please provide a search query, or a youtube video")
		}
	}
}

func cmdQueue(s *discordgo.Session, m *commandParameter) {

	str := "```"
	number := int(len(queue)/10) + 1

	for k, v := range queue {
		d := v.Base.Duration.Round(time.Second)
		m := int(math.Floor(d.Minutes()))
		s := int(d.Seconds()) % 60

		duration := fmt.Sprintf("%02d:%02d", m, s)

		format := "%0" + strconv.Itoa(number) + "d. %s [%s] | %s\n"
		str += fmt.Sprintf(format, k+1, v.Base.Title, duration, v.Name)
	}

	str += "```"

	s.ChannelMessageSend(m.ChannelID, str)
}

func cmdPing(s *discordgo.Session, m *commandParameter) {
	s.ChannelMessageSend(m.ChannelID, "ok")
}

func cmdSkip(s *discordgo.Session, m *commandParameter) {
	pause = false
	if queueindex >= 0 {
		i := queueindex + 1
		if i < len(queue) {
			setqueueindex(i)
		}
	}
}

func cmdLoop(s *discordgo.Session, m *commandParameter) {
	if len(m.Split) >= 2 {
		state := m.Split[1]
		if state == "off" {
			loop = loopOff
		} else if state == "song" {
			loop = loopSong
		} else if state == "playlist" || state == "queue" {
			loop = loopQueue
		}
	} else {
		loop = loop + 1
		if loop > loopQueue {
			loop = loopOff
		}
	}

	str := "cmdLoop( is set to **"
	if loop == loopOff {
		str += "off"
	} else if loop == loopSong {
		str += "current song"
	} else if loop == loopQueue {
		str += "current queue"
	}

	str += "**"

	s.ChannelMessageSend(m.ChannelID, str)
}

func cmdJoin(s *discordgo.Session, m *commandParameter) {
	channel, err := s.State.Channel(m.ChannelID)
	if err != nil {
		return
	}

	guild, err := s.State.Guild(channel.GuildID)
	if err != nil {
		return
	}

	for _, vs := range guild.VoiceStates {
		if vs.UserID == m.Author.ID {
			vc, _ = s.ChannelVoiceJoin(vs.GuildID, vs.ChannelID, false, true)
			return
		}
	}

	s.ChannelMessageSend(m.ChannelID, "You need to be in a voice channel")
}

func cmdPlaySample(s *discordgo.Session, m *commandParameter) {
	addsong := func(str string) {
		if len(m.Split) >= 2 {
			m.Split[1] = str
		} else {
			m.Split = append(m.Split, str)
		}

		cmdPlay(s, m)
	}

	for _, v := range sample {
		addsong(v)
	}
}

func cmdVolume(s *discordgo.Session, m *commandParameter) {
	if len(m.Split) >= 2 {
		vol, err := strconv.Atoi(m.Split[1])
		if err == nil {
			if vol <= 100 && vol >= 0 {
				volume = float64(vol) / 100
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Volume set to **%02d**", vol))
			} else {
				s.ChannelMessageSend(m.ChannelID, "Provided volume is less than 0 or more than 100")
			}
		}
	} else {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Current volume is set to **%02d**", int(volume*100)))
	}
}

func cmdPause(s *discordgo.Session, m *commandParameter) {
	if !pause {
		s.ChannelMessageSend(m.ChannelID, "Paused the current song")
	}

	pause = true
}

func cmdResume(s *discordgo.Session, m *commandParameter) {
	if pause {
		s.ChannelMessageSend(m.ChannelID, "Resume the current song")
	}

	pause = false
}

func cmdSetName(s *discordgo.Session, m *commandParameter) {
	name := ""

	if len(m.Split) >= 2 {
		for k, v := range m.Split[1:] {
			if k+1 != len(m.Split) {
				name += v + " "
			}
		}

		_, err := s.UserUpdate("", "", name, "", "")
		if err == nil {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Successfully changed the name from **%s** to **%s**", s.State.User.Username, name))
		} else {
			s.ChannelMessageSend(m.ChannelID, "You're changing the avatar too fast, Try again later.")
		}
	} else {
		s.ChannelMessageSend(m.ChannelID, "Please provide a name for me to change")
	}

}

func cmdSetAvatar(s *discordgo.Session, m *commandParameter) {
	avatar := ""

	if len(m.Attachments) > 0 {
		avatar = m.Attachments[0].URL
	} else {
		if len(m.Split) >= 2 {
			avatar = m.Split[1]
			_, err := url.ParseRequestURI(avatar)
			if err != nil {
				avatar = ""

				s.ChannelMessageSend(m.ChannelID, "Please provide an image with the message, or a link to an image")

				return
			}
		}
	}

	if avatar != "" {
		resp, err := http.Get(avatar)
		if err == nil {
			defer resp.Body.Close()
			img, err := ioutil.ReadAll(resp.Body)
			if err == nil {
				contentType := http.DetectContentType(img)
				base64img := base64.StdEncoding.EncodeToString(img)

				avatar := fmt.Sprintf("data:%s;base64,%s", contentType, base64img)
				s.UserUpdate("", "", "", avatar, "")

				s.ChannelMessageSend(m.ChannelID, "Successfully updated the avatar")
			}
		}
	}
}

func cmdShuffle(s *discordgo.Session, m *commandParameter) {
	shuffle = !shuffle
	if shuffle {
		s.ChannelMessageSend(m.ChannelID, "Shuffle is now **enabled**")
	} else {
		s.ChannelMessageSend(m.ChannelID, "Shuffle is now **disabled**")
	}

}

func cmdClear(s *discordgo.Session, m *commandParameter) {
	queue = []*videoInfo{}
	setqueueindex(-1)

	s.ChannelMessageSend(m.ChannelID, "Successfully cleared the queue")
}

/*func cmdSpeed(s *discordgo.Session, m *commandParameter) {
	if len(m.Split) >= 2 {
		speed, err := strconv.ParseFloat(m.Split[1], 32)
		if err == nil {
			if speed <= 2.0 && speed >= 0.25 {
				playbackspeed = int(math.Floor(speed * 100))
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Playback Speed set to **%02d**", playbackspeed))
			} else {
				s.ChannelMessageSend(m.ChannelID, "Provided playback speed is less than 0.25 or more than 2")
			}
		}
	} else {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Current playback speed is set to **%02d**", playbackspeed))
	}
}*/
