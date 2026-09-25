package service

import internallogger "github.com/SawaMEN/3x-ui/v3/internal/logger"

type serviceLoggerBridge struct{}

func (serviceLoggerBridge) Warningf(format string, args ...any) {
	internallogger.Warningf(format, args...)
}

var logger serviceLoggerBridge
