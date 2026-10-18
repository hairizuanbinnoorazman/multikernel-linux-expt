package safefile

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// ValidateScriptInterpreter checks the bounded shebang of a held script and
// every interpreter path it selects. The kernel still opens interpreters by
// pathname; their trusted directory ownership protects that selection from
// less-privileged replacement. Shared-library/imports remain a separate audit.
func ValidateScriptInterpreter(file *os.File, environment []string) error {
	return validateScriptInterpreter(file, environment, 0)
}

func validateScriptInterpreter(file *os.File, environment []string, depth int) error {
	if file == nil || depth > 4 {
		return errors.New("script interpreter is missing or recursion exceeds limit")
	}
	beforeInfo, err := file.Stat()
	before, ok := identity(beforeInfo)
	if err != nil || !ok || !beforeInfo.Mode().IsRegular() {
		return errors.New("script interpreter inspection requires a held regular file")
	}
	buffer := make([]byte, 256)
	count, err := unix.Pread(int(file.Fd()), buffer, 0)
	if err != nil {
		return err
	}
	buffer = buffer[:count]
	readInfo, err := file.Stat()
	readIdentity, ok := identity(readInfo)
	if err != nil || !ok || readIdentity != before {
		return errors.New("script identity changed during shebang read")
	}
	if !bytes.HasPrefix(buffer, []byte("#!")) {
		if bytes.HasPrefix(buffer, []byte{0x7f, 'E', 'L', 'F'}) {
			if err := validateELFLoader(file, before.Size, environment, depth); err != nil {
				return err
			}
			afterInfo, err := file.Stat()
			after, ok := identity(afterInfo)
			if err != nil || !ok || after != before {
				return errors.New("ELF identity changed during loader validation")
			}
		}
		return nil
	}
	end := bytes.IndexByte(buffer, '\n')
	if end < 0 {
		if len(buffer) == 256 {
			return errors.New("script shebang exceeds bounded header")
		}
		end = len(buffer)
	}
	header := strings.Trim(string(buffer[2:end]), " \t")
	if header == "" || strings.ContainsAny(header, "\x00\r") {
		return errors.New("script shebang is empty or contains unsupported controls")
	}
	interpreter, optional := header, ""
	if separator := strings.IndexAny(header, " \t"); separator >= 0 {
		interpreter, optional = header[:separator], strings.Trim(header[separator:], " \t")
	}
	opened, err := openTrustedInterpreter(interpreter, 256<<20)
	if err != nil {
		return fmt.Errorf("script interpreter path rejected: %w", err)
	}
	defer opened.Close()
	if err = validateScriptInterpreter(opened, environment, depth+1); err != nil {
		return err
	}
	if filepath.Base(interpreter) == "env" {
		// Linux passes the optional shebang argument as one string. Accept a
		// single command, never env options, assignments or split-string syntax.
		if optional == "" || strings.ContainsAny(optional, " \t=") || strings.HasPrefix(optional, "-") {
			return errors.New("env shebang requires one command without options or assignments")
		}
		command, err := openEnvironmentInterpreter(optional, environment)
		if err != nil {
			return err
		}
		defer command.Close()
		if err = validateScriptInterpreter(command, environment, depth+1); err != nil {
			return err
		}
	}
	afterInfo, err := file.Stat()
	after, ok := identity(afterInfo)
	if err != nil || !ok || after != before {
		return errors.New("script identity changed during interpreter validation")
	}
	return nil
}

func openEnvironmentInterpreter(command string, environment []string) (*os.File, error) {
	if filepath.IsAbs(command) {
		return openTrustedInterpreter(command, 256<<20)
	}
	if strings.ContainsRune(command, '/') {
		return nil, errors.New("relative env interpreter paths are prohibited")
	}
	search := ""
	for index := len(environment) - 1; index >= 0; index-- {
		if value, found := strings.CutPrefix(environment[index], "PATH="); found {
			search = value
			break
		}
	}
	if search == "" {
		return nil, errors.New("env interpreter requires an explicit PATH")
	}
	directories := filepath.SplitList(search)
	for _, directory := range directories {
		if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory {
			return nil, errors.New("env interpreter PATH must have canonical absolute nonempty entries")
		}
	}
	for _, directory := range directories {
		file, err := openTrustedInterpreter(filepath.Join(directory, command), 256<<20)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("env command path rejected: %w", err)
		}
		return file, nil
	}
	return nil, errors.New("env interpreter command is absent from explicit PATH")
}
