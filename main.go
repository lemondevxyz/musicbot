package main

import (
	"bufio"
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
	ytdl "github.com/kkdai/youtube/v2"
	"github.com/spf13/viper"
	"google.golang.org/api/googleapi/transport"
	"google.golang.org/api/youtube/v3"
	"gopkg.in/hraban/opus.v2"
)

type commandCallback func(s *discordgo.Session, m *commandParameter)
type commandParameter struct {
	*discordgo.MessageCreate
	cmd   *command
	Split []string
}

//

type videoInfo struct {
	Base *ytdl.Video
	Name string
}

type command struct {
	alias    []string
	help     string
	messages map[string]string
	callback commandCallback
}

var sesh *discordgo.Session

var commands []*command
var ytcl = ytdl.Client{
	Debug: false,
}

func init() {
	commands = []*command{
		&command{
			alias: []string{"play", "pl"},
			help:  "Adds a song to the queue",
			messages: map[string]string{
				"success": "Added **{{title}}** to the queue!",
				"empty":   "No videos found",
				"param":   "Please provide a serach query, or a link to a youtube video",
			},
			callback: cmdPlay,
		},

		&command{
			alias: []string{"queue", "q", "playlist"},
			help:  "Sends a message containing the songs in the current",
			messages: map[string]string{
				"start": "```",
				"loop":  "{{index}}. {{title}} | {{name}}",
				"end":   "```",
				"empty": "The queue is empty",
			},
			callback: cmdQueue,
		},

		&command{
			alias: []string{"skip", "sk"},
			help:  "Skips the current song and plays the next song if there is one",
			messages: map[string]string{
				"skip": "Skipped the last song",
			},
			callback: cmdSkip,
		},

		&command{
			alias: []string{"loop", "l"},
			help:  "Sets the loop mode to one of three: off, current song, current queue",
			messages: map[string]string{
				"off":   "Current loop is set to **off**",
				"song":  "Current loop is set to **current song**",
				"queue": "Current loop is set to **current queue**",
			},
			callback: cmdLoop,
		},

		&command{
			alias: []string{"join", "j"},
			help:  "Joins the current voice channel",
			messages: map[string]string{
				"already_in": "I am already in a voice channel",
				"no_channel": "You need to be in a voice channel",
			},
			callback: cmdJoin,
		},

		&command{
			alias: []string{"volume", "vol", "v"},
			help:  "Displays or sets the volume, acceptable values are from 0 to 100",
			messages: map[string]string{
				"volume": "Volume is set to **{{volume}}**",
			},
			callback: cmdVolume,
		},

		&command{
			alias: []string{"pause", "pa"},
			help:  "Pauses the current song",
			messages: map[string]string{
				"pause": "Paused the current song",
			},
			callback: cmdPause,
		},

		&command{
			alias: []string{"resume", "re"},
			help:  "Resumes the current song",
			messages: map[string]string{
				"resume": "Resumed the current song",
			},
			callback: cmdResume,
		},

		&command{
			alias: []string{"setname", "sn"},
			help:  "Sets the bot's name",
			messages: map[string]string{
				"setname":   "Change the bot's name from **{{old}}** to **{{new}}**",
				"ratelimit": "You're changing the avatar too fast, Try again later.",
				"param":     "Please provide a name for me to change",
			},
			callback: cmdSetName,
		},

		&command{
			alias: []string{"setavatar", "sa"},
			help:  "Sets the bot's avatar",
			messages: map[string]string{
				"setavatar": "Successfully changed the bot's profile picture",
				"param":     "Please provide an image as attachment, or a url of an image",
			},
			callback: cmdSetAvatar,
		},

		&command{
			alias: []string{"shuffle", "sh"},
			help:  "Enables or Disables shuffle mode",
			messages: map[string]string{
				"on":  "Shuffle is now **on**",
				"off": "Shuffle is now **off**",
			},
			callback: cmdShuffle,
		},

		&command{
			alias: []string{"help", "h"},
			help:  "Send a message explaining every command",
			messages: map[string]string{
				"start":      "",
				"startalias": "[",
				"cmd":        "**{{name}}** *{{alias}}*\n{{help}}\n", // name is the first alias
				"endalias":   "]",
				"end":        "",
			},
			callback: cmdHelp,
		},

		&command{
			alias: []string{"clear", "c"},
			help:  "Clears the queue",
			messages: map[string]string{
				"start": "```",
				"clear": "Successfully cleared the queue",
				"end":   "```",
			},
			callback: cmdClear,
		},

		&command{
			alias: []string{"leave", "l"},
			help:  "Leaves the voice channel",
			messages: map[string]string{
				"novoice": "The bot is not currently inside a voice channel",
			},
			callback: cmdLeave,
		},
	}
}

var yt *youtube.Service
var vc *discordgo.VoiceConnection

var queue = []*videoInfo{}
var queueindex = -1 // This is the original queue index

var loop = loopOff
var shuffle = false

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

	viper.SetConfigName("config")
	viper.AddConfigPath(".")
	viper.SetDefault("botToken", "")
	viper.SetDefault("youtubeKey", "")
	viper.SetDefault("prefix", "")
	viper.SetDefault("status", "")

	var err error

	// Initiate viper for our config
	err = viper.ReadInConfig()
	if err != nil {
		log.Fatalf("Cannot read config, error: %v", err)
	}

	// Unmarshal the config
	err = viper.Unmarshal(&config)
	if err != nil {
		log.Fatalf("Unable to unmarshal config, error: %v", err)
	}

	// OpusEncoder is used to encode the Output file to discord's own DCA format
	opusEncoder, err = opus.NewEncoder(audioFrameRate, audioChannels, opus.AppAudio)
	if err != nil {
		log.Fatalf("An error occured with initializing the OpusEncoder, error: %v", err)
	}

	opusEncoder.SetBitrate(audioBitrate * 1000)

	// This goroutine manages the newly-added songs, whenever a new song is added it calls
	// send() to stream it to discord, and then cleans the file for another song to be played.
	// If there are any problems with the queue, most likely it's from this function alone.
	go func() {

		for {
			if len(queue) > queueindex && queueindex >= 0 {

				if pause {
					pause = false
				}

				vid := queue[queueindex]
				playingAudio = true
				// close

				var format *ytdl.Format
				minsize := int64(0)
				for _, v := range vid.Base.Formats.Type("audio") {
					if format == nil || (format != nil && minsize > v.ContentLength) {
						format = &v
						fmt.Println("hi", format.MimeType, v.ContentLength)
						minsize = v.ContentLength
					}
				}

				fmt.Println(len(vid.Base.Formats), vid.Base.Formats[0].MimeType)
				fmt.Println("last", format.MimeType)
				dl, _, err := ytcl.GetStream(vid.Base, format)
				if err != nil {
					log.Fatalf("ytcl.GetStream: %s", err.Error())
				}

				rd := bufio.NewReaderSize(dl, bufferSize)
				time.Sleep(time.Second)

				send(rd)
				dl.Close()
			}

			time.Sleep(time.Second)
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

	if len(config.Status) > 0 {
		fmt.Println(sesh.UpdateListeningStatus(config.Status))
	}

	client := &http.Client{
		Transport: &transport.APIKey{Key: config.YoutubeKey},
	}

	yt, err = youtube.New(client)
	if err != nil {
		log.Fatalf("Error creating new YouTube client: %v", err)
	}

	log.Println("Session created successfully")
	sig := make(chan os.Signal)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM, os.Kill, syscall.SIGINT)
	<-sig

	// Close the session
	sesh.Close()
	log.Println("Closed Session")
}

func setqueueindex(v int) {
	if len(queue) >= v {
		queueindex = v
	}
}

func replacestringwithtrackinfo(str string, track *videoInfo) string {

	base := track.Base
	d := base.Duration.Round(time.Second)
	m := int(math.Floor(d.Minutes()))
	s := int(d.Seconds()) % 60

	duration := fmt.Sprintf("%02d:%02d", m, s)

	replaces := strings.NewReplacer(
		"{{title}}", base.Title,
		"{{id}}", base.ID,
		"{{description}}", base.Description,
		"{{publishdate}}", base.PublishDate.Format("2006/01/02"),
		"{{author}}", base.Author,
		"{{duration}}", duration,
		"{{name}}", track.Name)

	return replaces.Replace(str)
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
		var cmd *command
		for _, v := range commands {
			for _, alias := range v.alias {
				if alias == split[0] {
					cmd = v
					break
				}
			}
		}

		if cmd != nil {
			cp := &commandParameter{
				m,
				cmd,
				split,
			}

			if cmd.callback != nil {
				go cmd.callback(s, cp)
			}
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

			call := yt.Search.List([]string{"id"}).Q(urlstring).MaxResults(1).Type("video")

			res, err := call.Do()
			if err != nil {
				return
				//s.ChannelMessageSend(m.ChannelID, "An error error occured with searching for the video, please consult @toms#1441")
			}

			if len(res.Items) > 0 {
				yturl = "https://youtube.com/watch?v=" + res.Items[0].Id.VideoId
			}
		}

		if len(yturl) > 0 {
			vid, err := ytcl.GetVideo(yturl)
			if err == nil {

				newvid := &videoInfo{
					Base: vid,
					Name: "@" + m.Author.String(),
				}

				oldlen := len(queue)
				queue = append(queue, newvid)

				s.ChannelMessageSend(m.ChannelID, replacestringwithtrackinfo(m.cmd.messages["success"], newvid))

				if vc == nil {
					cmdJoin(s, m)
				}

				// If we have a clear queue, set queueindex to 0 to initiate the first song.
				if queueindex < 0 && oldlen == 0 {
					setqueueindex(0)
				}
			}
		} else {
			s.ChannelMessageSend(m.ChannelID, m.cmd.messages["empty"])
		}
	} else {
		if pause {
			cmdResume(s, m)
		} else {
			setqueueindex(queueindex)
		}
	}
}

func cmdQueue(s *discordgo.Session, m *commandParameter) {

	var str string
	if len(queue) > 0 {
		str = m.cmd.messages["start"]

		var i = queueindex
		var start, end int

		if len(queue) > i+25 || len(queue) == i+25 {
			start = i
			end = i + 25
		} else if len(queue) < i+25 {
			start = i - (i + 25 - len(queue))
			end = len(queue)
		}

		if start < 0 {
			start = 0
		}

		/*
			Three cases:
			First case:
				50 songs and current song is 15.
				so we need to display starting from 15 to 40.
			Second case:
				50 songs and current song is 35.
				so we need to start displaying from 25 to 50.
			Third case(runs first condition):
				50 seconds and current song is 25
				so we need to start from the 25 to 50.
		*/

		for i = start; i < end; i++ {
			if len(queue) > i {
				v := queue[i]

				if v != nil {

					newstr := replacestringwithtrackinfo(m.cmd.messages["loop"], v)
					newstr = strings.ReplaceAll(newstr, "{{index}}", fmt.Sprintf("%02d", i+1))

					str += newstr

					if i+1 != len(queue) {
						str += "\n"
					}
				}
			}
		}

		str += m.cmd.messages["end"]
	} else {
		str = m.cmd.messages["empty"]
	}

	s.ChannelMessageSend(m.ChannelID, str)
}

func cmdSkip(s *discordgo.Session, m *commandParameter) {
	pause = false
	if queueindex >= 0 {
		i := queueindex + 1
		if i <= len(queue) {
			setqueueindex(i)

			if len(m.cmd.messages["skip"]) > 0 {
				s.ChannelMessageSend(m.ChannelID, m.cmd.messages["skip"])
			}
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

	str := ""
	if loop == loopOff {
		str = m.cmd.messages["off"]
	} else if loop == loopSong {
		str = m.cmd.messages["song"]
	} else if loop == loopQueue {
		str = m.cmd.messages["queue"]
	}

	s.ChannelMessageSend(m.ChannelID, str)
}

func cmdJoin(s *discordgo.Session, m *commandParameter) {

	if vc != nil {
		s.ChannelMessageSend(m.ChannelID, m.cmd.messages["already_in"])

	}

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

	s.ChannelMessageSend(m.ChannelID, m.cmd.messages["no_voice"])
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
			}
		}
	}

	str := m.cmd.messages["volume"]
	str = strings.ReplaceAll(str, "{{volume}}", fmt.Sprintf("%02d", int(volume*100)))

	s.ChannelMessageSend(m.ChannelID, str)
}

func cmdPause(s *discordgo.Session, m *commandParameter) {
	if !pause {
		s.ChannelMessageSend(m.ChannelID, m.cmd.messages["pause"])
	}

	pause = true
}

func cmdResume(s *discordgo.Session, m *commandParameter) {
	if pause {
		s.ChannelMessageSend(m.ChannelID, m.cmd.messages["resume"])
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
			s.ChannelMessageSend(m.ChannelID, m.cmd.messages["setname"])
		} else {
			s.ChannelMessageSend(m.ChannelID, m.cmd.messages["ratelimit"])
		}
	} else {
		s.ChannelMessageSend(m.ChannelID, m.cmd.messages["param"])
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

				s.ChannelMessageSend(m.ChannelID, m.cmd.messages["param"])

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

				s.ChannelMessageSend(m.ChannelID, m.cmd.messages["setavatar"])
			}
		}
	}
}

func cmdShuffle(s *discordgo.Session, m *commandParameter) {
	shuffle = !shuffle
	if shuffle {
		s.ChannelMessageSend(m.ChannelID, m.cmd.messages["on"])
	} else {
		s.ChannelMessageSend(m.ChannelID, m.cmd.messages["off"])
	}

}

func cmdClear(s *discordgo.Session, m *commandParameter) {
	queue = []*videoInfo{}
	setqueueindex(-1)

	s.ChannelMessageSend(m.ChannelID, m.cmd.messages["clear"])
}

func cmdHelp(s *discordgo.Session, m *commandParameter) {

	str := m.cmd.messages["start"]

	for _, v := range commands {

		name := v.alias[0]
		name = strings.Title(name)

		alias := m.cmd.messages["startalias"]
		for k, val := range v.alias {
			alias += val
			if k+1 < len(v.alias) {
				alias += ","
			}
		}
		alias += m.cmd.messages["endalias"]

		format := m.cmd.messages["cmd"]

		format = strings.ReplaceAll(format, "{{alias}}", alias)
		format = strings.ReplaceAll(format, "{{name}}", name)
		format = strings.ReplaceAll(format, "{{help}}", v.help)

		str += format
	}

	str += m.cmd.messages["end"]

	chn, err := s.UserChannelCreate(m.Author.ID)
	if err == nil {
		s.ChannelMessageSend(chn.ID, str)
	}
}

func cmdLeave(s *discordgo.Session, m *commandParameter) {
	if vc != nil {

		cmdSkip(s, m)

		go func() {
			time.Sleep(time.Millisecond * 50)
			vc.Disconnect()
			vc = nil
		}()

	} else {
		s.ChannelMessageSend(m.ChannelID, m.cmd.messages["novoice"])
	}
}
