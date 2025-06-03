package main

import (
	"fmt"
	"io"
)

const (
	ResetColor  = "\033[0m"
	ClearScreen = "\033[2J\033[H"
)

type ColoredWriter struct {
	writer io.Writer
	color  []byte
	reset  []byte
}

func NewColoredWriter(w io.Writer, color string) ColoredWriter {
	return ColoredWriter{
		writer: w,
		color:  []byte(color),
		reset:  []byte(ResetColor),
	}
}

func (w ColoredWriter) Write(p []byte) (n int, err error) {
	n, err = w.writer.Write(w.color)
	if err != nil {
		return
	}
	return w.writer.Write(p)
}

func rgb(r, g, b int) string {
	return fmt.Sprintf("\033[38;2;%d;%d;%dm", r, g, b)
}

func sprintRGB(r, g, b int, text string) string {
	return fmt.Sprintf("%s%s%s", rgb(r, g, b), text, ResetColor)
}

func clearScreen() {
	fmt.Print(ClearScreen)
}
