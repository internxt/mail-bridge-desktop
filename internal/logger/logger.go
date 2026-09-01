package logger

import (
	"log"
	"os"
)

func New(scope string) *Logger {
	return &Logger{l: log.New(os.Stderr, "["+scope+"] ", log.LstdFlags|log.Lmsgprefix)}
}

func (lg *Logger) Info(format string, a ...any)  { lg.l.Printf("INFO  "+format, a...) }
func (lg *Logger) Warn(format string, a ...any)  { lg.l.Printf("WARN  "+format, a...) }
func (lg *Logger) Error(format string, a ...any) { lg.l.Printf("ERROR "+format, a...) }
