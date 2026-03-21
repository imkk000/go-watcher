# Go Watcher

- Use this as hack module name: `watcher`
- Original name is `go-watcher`, but create link to `~/.bin/hack-watcher`
- Implement `completion json`, so override built-in urfave cli completion
- Using `SIGKILL` instead of `SIGTERM` to terminate process, so I have to handle zombie process too

## Command

```sh
go-watcher watch fs --env=. -- go run .
go-watcher watch cmd -- go run .
```
