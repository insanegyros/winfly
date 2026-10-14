package diskimage

import (
	"errors"
	"strings"
)

var errUnterminatedQuote = errors.New("neukončené uvozovky")

// splitCommandLine rozdělí příkaz z autorun.inf na program a parametry podle
// pravidel Windows (CommandLineToArgvW):
//
//   - program končí první mezerou, nebo je celý v uvozovkách,
//   - parametry v uvozovkách mohou obsahovat mezery,
//   - 2n zpětných lomítek před uvozovkou znamená n lomítek a uvozovku jako
//     hranici, 2n+1 lomítek znamená n lomítek a uvozovku jako znak.
//
// Na rozdíl od Windows se neukončené uvozovky odmítají jako chyba.
func splitCommandLine(command string) (program string, args []string, err error) {
	command = strings.TrimLeft(command, " \t")
	program, rest, err := splitProgram(command)
	if err != nil {
		return "", nil, err
	}
	args, err = splitArguments(rest)
	if err != nil {
		return "", nil, err
	}
	return program, args, nil
}

// splitProgram oddělí název programu; v něm se zpětná lomítka berou doslova.
func splitProgram(command string) (program, rest string, err error) {
	if !strings.HasPrefix(command, `"`) {
		end := strings.IndexAny(command, " \t")
		if end < 0 {
			return command, "", nil
		}
		return command[:end], command[end+1:], nil
	}
	end := strings.IndexByte(command[1:], '"')
	if end < 0 {
		return "", "", errUnterminatedQuote
	}
	return command[1 : end+1], command[end+2:], nil
}

func splitArguments(rest string) ([]string, error) {
	args := []string{}
	var current strings.Builder
	inArgument := false
	inQuotes := false
	backslashes := 0

	for _, r := range rest {
		switch {
		case r == '\\':
			backslashes++
			inArgument = true
			continue
		case r == '"':
			current.WriteString(strings.Repeat(`\`, backslashes/2))
			if backslashes%2 == 1 {
				current.WriteRune('"')
			} else {
				inQuotes = !inQuotes
			}
			backslashes = 0
			inArgument = true
			continue
		}

		current.WriteString(strings.Repeat(`\`, backslashes))
		backslashes = 0
		isSeparator := (r == ' ' || r == '\t') && !inQuotes
		switch {
		case isSeparator && inArgument:
			args = append(args, current.String())
			current.Reset()
			inArgument = false
		case !isSeparator:
			current.WriteRune(r)
			inArgument = true
		}
	}

	if inQuotes {
		return nil, errUnterminatedQuote
	}
	current.WriteString(strings.Repeat(`\`, backslashes))
	if inArgument {
		args = append(args, current.String())
	}
	return args, nil
}
