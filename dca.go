package main

import (
	"bufio"
	"encoding/binary"
	"log"
	"math"
	"math/rand"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"layeh.com/gopus"
)

var (

	// AudioChannels sets the ops encoder channel value.
	// Must be set to 1 for mono, 2 for stereo
	audioChannels = 2

	// AudioFrameRate sets the opus encoder Frame Rate value.
	// Must be one of 8000, 12000, 16000, 24000, or 48000.
	// Discord only uses 48000 currently.
	audioFrameRate = 48000

	// AudioBitrate sets the opus encoder bitrate (quality) value.
	// Must be within 500 to 512000 bits per second are meaningful.
	// Discord only uses 8000 to 128000 and default is 64000.
	audioBitrate = 64

	// AudioApplication sets the opus encoder Application value.
	// Must be one of voip, audio, or lowdelay.
	// DCA defaults to audio which is ideal for music.
	// Not sure what Discord uses here, probably voip.
	audioApplication = "audio"

	// AudioFrameSize sets the opus encoder frame size value.
	// The Frame Size is the length or amount of milliseconds each Opus frame
	// will be.
	// Must be one of 960 (20ms), 1920 (40ms), or 2880 (60ms)
	audioFrameSize = 960

	// AudioFilename is the filename for the song to be downloaded to.
	audioFilename = "song.mp3"
	// DcaFilename is the filename that takes audioFilename and converts it to dca
	dcaFilename = "song.dca"

	// maxBytes is a calculated value of the largest possible size that an
	// opus frame could be.
	maxBytes = (audioFrameSize * audioChannels)

	// opusEncoder holds an instance of an gopus Encoder
	opusEncoder *gopus.Encoder

	// waitGroup is used to wait until all goroutines have finished.
	waitGroup sync.WaitGroup

	encodeChan chan []int16

	// The perception of loudness from the intensity of the sound waves.
	volume = float64(1.0)

	// The Rate at which the audio file is able to read.
	// playbackspeed = 100

	// Send channel is used to measure when queueindex has been changed
	sendchannel = getchan()
)

func convert() error {

	ff := exec.Command("ffmpeg", []string{
		"-y",
		"-i", audioFilename,
		"-f", "s16le",
		"-ar", strconv.Itoa(audioFrameRate),
		"-ac", strconv.Itoa(audioChannels),
		dcaFilename,
	}...)

	return ff.Run()
}

/*
func read() {

	var err error

	waitGroup.Add(1)

	defer func() {
		close(encodeChan)
		waitGroup.Done()
	}()

	f, err := os.Open("sample.opus")
	if err != nil {
		return
	}

	// Create a 16KB input buffer
	filebuffer := bufio.NewReaderSize(f, 16384)

	// Loop over the stdin input and pass the data to the encoder.
	for {

		buf := make([]int16, maxBytes)

		err = binary.Read(filebuffer, binary.LittleEndian, &buf)
		if err == io.EOF {
			// Okay! There's nothing left, time to quit.
			return
		}

		if err == io.ErrUnexpectedEOF {
			// Well there's just a tiny bit left, lets encode it, then quit.
			encodeChan <- buf
			return
		}

		if err != nil {
			// Oh no, something went wrong!
			log.Println("error reading from file buffer", err)
			return
		}

		for k := range buf {
			buf[k] = int16(math.Floor(float64(buf[k]) * volume)) // Should work +/- values
		}

		// write pcm data to the encodeChan
		encodeChan <- buf
	}
}
*/

// send reads from the converted opus file, then sends it to the voice connection
func send() {

	if err := convert(); err != nil {
		log.Println(err)
		return
	}

	qi := queueindex

	defer func(qi int) {

		if vc != nil {
			vc.Speaking(false)
		}

		if qi == queueindex {

			if shuffle == false && loop != loopSong {
				if len(queue) < queueindex && loop != loopSong {
					setqueueindex(queueindex + 1)
				}

				if queueindex == len(queue) && loop == loopQueue {
					setqueueindex(0)
				}
			} else if shuffle == true && loop != loopSong {
				arr := []int{}
				for i := 0; i < len(queue); i++ {
					if i == queueindex {
						continue
					}

					arr = append(arr, i)
				}

				rand.Seed(time.Now().UnixNano())
				if len(arr) > 1 {
					setqueueindex(arr[rand.Intn(len(arr)-1)])
				}
			}

			if loop == loopSong {
				setqueueindex(qi)
			}

		}

	}(qi)

	if vc != nil {
		vc.Speaking(true)
	}

	// Create a 16KB input buffer, old: 16384 - new:
	filebuffer := bufio.NewReaderSize(output, 16384)
	var err error

	for {
		select {
		case <-sendchannel:
			return
		default:
			if pause {
				continue
			}

			buf := make([]int16, maxBytes)

			err = binary.Read(filebuffer, binary.LittleEndian, &buf)
			if err != nil {
				// Okay! There's nothing left, time to quit.
				return
			}

			for k := range buf {
				buf[k] = int16(math.Floor(float64(buf[k]) * volume)) // Should work +/- values
			}

			opus, err := opusEncoder.Encode(buf, audioFrameSize, maxBytes)
			if err == nil {
				if vc != nil {
					vc.OpusSend <- opus
				}
			}

		}

	}

}
