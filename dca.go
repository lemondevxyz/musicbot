package main

import (
	"bufio"
	"encoding/binary"
	"io"
	"log"
	"math"
	"math/rand"
	"os/exec"
	"strconv"
	"sync"

	"gopkg.in/hraban/opus.v2"
)

var (
	// BufferSize is used when downloading the youtube video so that ffmpeg doesn't over-reach while transcoding. Currently set to 512 KB.
	bufferSize = 1024 * 512
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
	originalMaxBytes = (audioFrameSize * audioChannels)

	// opusEncoder holds an instance of an gopus Encoder
	opusEncoder *opus.Encoder

	// waitGroup is used to wait until all goroutines have finished.
	waitGroup sync.WaitGroup

	encodeChan chan []int16

	// The perception of loudness from the intensity of the sound waves.
	volume = float64(1.0)

	// The Rate at which the audio file is able to read.
	playbackspeed = 100

	// playingAudio is used to determine if currently a song is playing or not. It starts from the goroutine in main()
	playingAudio bool
)

// send reads from the converted opus file, then sends it to the voice connection
func send(input io.Reader) {
	qi := queueindex
	defer func(qi int) {
		if vc != nil {
			vc.Speaking(false)
		}

		//log.Println("ran defer")
		playingAudio = false

		// If the user didn't skip the song
		if qi == queueindex {

			// If shuffle is off and loop is not set to loop song
			if shuffle == false && loop != loopSong {
				// If the amount of songs exceeds the current song index, i.e
				// amount of songs: 5, current song: 4, queueindex would become 5
				if len(queue) > queueindex {
					setqueueindex(queueindex + 1)
				}

				// If the amount of songs is equal to the current song index, i.e
				// amount of songs: 5, current song: 5, queueindex would become 0. starting over again.
				if queueindex == len(queue) && loop == loopQueue {
					setqueueindex(0)
				}
				// If shuffle is off and loop is not set to loop song
			} else if shuffle == true && loop != loopSong {
				// Array to hold all the songs' index that are left
				arr := []int{}
				for i := 0; i < len(queue); i++ {
					if i == queueindex {
						continue
					}

					arr = append(arr, i)
				}

				// If there are more than 1 song in the array
				if len(arr) > 1 {
					// Set queue index to a random index that is equal to len(arr)-1
					// this will give us a random song index that is not the song that has been played before.
					setqueueindex(arr[rand.Intn(len(arr)-1)])
				}
			}

			// If loop is set to loop song then replay it
			if loop == loopSong {
				setqueueindex(qi)
			}

		}

	}(qi)

	if vc != nil {
		vc.Speaking(true)
	}

	ff := exec.Command("ffmpeg", []string{
		"-y",
		"-i", "-",
		"-f", "s16le",
		"-ar", strconv.Itoa(audioFrameRate),
		"-ac", strconv.Itoa(audioChannels),
		"-",
	}...)

	ff.Stdin = input
	pipe, err := ff.StdoutPipe()
	defer pipe.Close()
	defer func(ff *exec.Cmd) {
		if ff.Process != nil {
			ff.Process.Kill()
		}
	}(ff)
	if err != nil {
		log.Fatalf("ff.PipeStdout: %s", err.Error())
	}
	go ff.Start()
	go ff.Wait()

	output := bufio.NewReaderSize(pipe, bufferSize)

	// Create a 16KB input buffer
	filebuffer := bufio.NewReaderSize(output, 16384)

	for vc != nil {
		if queueindex != qi {
			break
		} else {
			if pause {
				continue
			}

			buf := make([]int16, originalMaxBytes)

			err := binary.Read(filebuffer, binary.LittleEndian, &buf)
			if err != nil {
				// Okay! There's nothing left, time to quit.
				break
			}

			for k := range buf {
				buf[k] = int16(math.Floor(float64(buf[k]) * volume)) // Should work +/- values
			}

			opus := make([]byte, originalMaxBytes)

			num, err := opusEncoder.Encode(buf, opus)
			if err == nil && num > 0 {
				vc.OpusSend <- opus[:num]
			} else {
				break
			}
		}
	}

}
