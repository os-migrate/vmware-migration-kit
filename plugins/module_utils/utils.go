/*
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Copyright 2024 Red Hat, Inc.
 *
 */

package moduleutils

import (
	"crypto/rand"
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func FindDevName(volumeID string) (string, error) {
	return FDevName(os.ReadDir, filepath.EvalSymlinks, volumeID)
}

func FDevName(
	readDir func(string) ([]fs.DirEntry, error),
	evalSymlinks func(string) (string, error),
	volumeID string,
) (string, error) {
	if len(volumeID) < 18 {
		return "", fmt.Errorf("volumeID must be at least 18 characters long")
	}

	files, err := readDir("/dev/disk/by-id/")
	if err != nil {
		return "", err
	}

	for _, file := range files {
		if strings.Contains(file.Name(), volumeID[:18]) {
			devicePath, err := evalSymlinks(filepath.Join("/dev/disk/by-id/", file.Name()))
			if err != nil {
				return "", err
			}
			return devicePath, nil
		}
	}
	return "", nil
}

func GenRandom(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		result[i] = charset[num.Int64()]
	}
	return string(result), nil
}

// transliterations maps common non-ASCII characters to their ASCII equivalents.
// Covers Spanish, Portuguese, French, German, and extended Latin characters.
var transliterations = map[rune]string{
	// Spanish
	'á': "a", 'é': "e", 'í': "i", 'ó': "o", 'ú': "u",
	'Á': "A", 'É': "E", 'Í': "I", 'Ó': "O", 'Ú': "U",
	'ñ': "n", 'Ñ': "N", 'ü': "u", 'Ü': "U",
	// Portuguese
	'ã': "a", 'õ': "o", 'à': "a", 'â': "a",
	'ê': "e", 'ô': "o", 'ç': "c",
	'Ã': "A", 'Õ': "O", 'À': "A", 'Â': "A",
	'Ê': "E", 'Ô': "O", 'Ç': "C",
	// French
	'è': "e", 'ë': "e", 'î': "i", 'ï': "i",
	'ù': "u", 'û': "u",
	'È': "E", 'Ë': "E", 'Î': "I", 'Ï': "I",
	'Ù': "U", 'Û': "U",
	'æ': "ae", 'Æ': "AE", 'œ': "oe", 'Œ': "OE",
	// German
	'ä': "a", 'ö': "o",
	'Ä': "A", 'Ö': "O",
	'ß': "ss",
	// Extended Latin
	'ì': "i", 'ò': "o",
	'Ì': "I", 'Ò': "O",
	// Common special characters
	'·':    "_", // interpunct
	'\u2019': "_", // right single quotation mark
	'\u2013': "_", // en-dash
	'\u2014': "_", // em-dash
	'\u00A0': "_", // non-breaking space
}

// transliterate replaces known non-ASCII runes with their ASCII equivalents.
func transliterate(vmName string) string {
	var b strings.Builder
	for _, r := range vmName {
		if replacement, ok := transliterations[r]; ok {
			b.WriteString(replacement)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SafeVmName sanitizes a VMware VM name for use as an OpenStack resource name.
// It transliterates common non-ASCII characters, replaces any remaining
// non-alphanumeric characters (except underscore) with underscores, collapses
// consecutive underscores into one, truncates to 64 characters, and strips
// any trailing underscores left after truncation.
func SafeVmName(vmName string) string {
	safe := transliterate(vmName)
	re := regexp.MustCompile(`[^a-zA-Z0-9_]`)
	safe = re.ReplaceAllString(safe, "_")
	multi := regexp.MustCompile(`_+`)
	safe = multi.ReplaceAllString(safe, "_")
	if len(safe) > 64 {
		safe = safe[:64]
	}
	safe = strings.TrimRight(safe, "_")
	return safe
}
