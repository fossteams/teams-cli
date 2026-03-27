package main

import (
	"fmt"
	"strconv"
	"strings"
)

const defaultMessageLimit = 200

type AppOptions struct {
	MessageLimit int
}

func defaultAppOptions() AppOptions {
	return AppOptions{
		MessageLimit: defaultMessageLimit,
	}
}

func parseAppOptions(args []string) (AppOptions, error) {
	options := defaultAppOptions()

	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "msg="):
			limit, err := parseMessageLimit(strings.TrimPrefix(arg, "msg="))
			if err != nil {
				return options, err
			}
			options.MessageLimit = limit
		case strings.HasPrefix(arg, "--msg="):
			limit, err := parseMessageLimit(strings.TrimPrefix(arg, "--msg="))
			if err != nil {
				return options, err
			}
			options.MessageLimit = limit
		default:
			return options, fmt.Errorf("unknown argument %q", arg)
		}
	}

	return options, nil
}

func parseMessageLimit(raw string) (int, error) {
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return 0, fmt.Errorf("invalid msg value %q: expected a positive integer", raw)
	}

	return limit, nil
}
