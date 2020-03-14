# musicbot
This is a music bot for discord made using golang. Priorities are high-performance and low footprint.


## Commands
- Play: Adds a song to the queue via URL or search query
- Ping: Tests the messagehandler, most likely will be removed in the future
- Queue: Outputs the current queue
- Skip: Skips the current song, and plays the next one
- Loop: Switches between three modes: off, current song, current queue
- Join: Joins the voice channel that the user is in
- Volume: Outputs the volume if there are 0 arguments, or sets the volume if there are arguments.
- Pause: Pauses the current song
- Resume: Resumes the current song
- Playsample: This tests the play command, most likely will be removed in the future
- Setname: Sets the name of the bot
- Setavatar: Sets the avatar of the bot
- Shuffle: Shuffles between songs, when loop is off
- Clear: Clears the current queue

## Performance
This bot uses I/O instead of Memory to store files, in-order to save memory and because reading a music file isn't that I/O-intensive.

The process of playing a song goes as followed:
First, it downloads the file of the video to `song.mp3`.
Second, it uses ffmpeg to standardize any audio file to later convert to discord's own audio encoding.
Third, it converts the ffmpeg file called `song.dca`, to discord-specific audio encoding and sends it at the same time.

After a song is done playing, the files are then emptied. The reason we don't delete them is we would have to allocate new file pointers.

## Dependencies
- ffmpeg(runtime)
- golang(build time)

## Building
Current this music bot supports only linux, if you are running windows or mac os it won't work for you. This is due to using the [gopus](github.com/layeh/gopus).
```
go get -u # Get all the golang dependencies
go build # Build the binary
```

## Installation
You can download one of the pre-built binaries, and do:
```
chmod +x music
./music
```

## Configuration
Configuration is done through the config file, possible file extensions are: `json`, `toml`, `yaml`, `hcl`, `envfile`.

Current values to set are:
- `botToken`: Discord's bot token, you can get your own bot token through this [link](https://github.com/reactiflux/discord-irc/wiki/Creating-a-discord-bot-&-getting-a-token)
- `youtubeKey`: This is used to search for videos from youtube, you can get your youtube api key through this [link](https://developers.google.com/youtube/v3/getting-started)
- `prefix`: This is the prefix to indicate which messages are meant to be commands.