// Developed by Rui Lopes (ruilopes.com). Released under the LGPLv3 license.

package nmea

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

type visitor struct {
	result interface{}
}

func (v *visitor) OnBeforeParse(sentenceType, sentence string) bool {
	v.result = nil
	return true
}

func (v *visitor) OnAfterParse(sentenceType, sentence string, err error) {}

func (v *visitor) OnGPGGA(gpgga *GPGGA) {
	v.result = gpgga
}

func (v *visitor) OnGPRMC(gprmc *GPRMC) {
	v.result = gprmc
}

func (v *visitor) OnGPGSA(gpgsa *GPGSA) {
	v.result = gpgsa
}

func (v *visitor) visit(sentence string) (interface{}, error) {
	err := Visit(strings.NewReader(sentence), v)
	return v.result, err
}

type validSentence struct {
	sentence string
	expected interface{}
}

func duration(text string) (result time.Duration) {
	result, err := time.ParseDuration(text)

	if err != nil {
		panic(fmt.Sprintf("failed to parse time `%s`: %v", text, err))
	}

	return
}

func checksum(sentence string) string {
	l := len(sentence)

	if l < 1 || sentence[0] != '$' {
		return "__"
	}

	if sentence[l-1] == '*' {
		l--
	} else {
		l -= 3
	}

	checksum := byte(0)

	for i := 1; i < l; i++ {
		checksum = checksum ^ byte(sentence[i])
	}

	return strings.ToUpper(hex.EncodeToString([]byte{checksum}))
}

var validSentences = []validSentence{
	//
	// GPGGA

	// before a fix.
	validSentence{
		"$GPGGA,064951.123,,,,,0,0,,,M,,M,,*47",
		&GPGGA{
			Time:           duration("6h49m51s123ms"),
			UsedSatellites: 0,
			PositionFix:    0,
			Latitude:       0,
			Longitude:      0,
			HDOP:           0,
			Altitude:       0}},

	// after a fix.
	validSentence{
		"$GPGGA,064951.000,2307.1256,N,12016.4438,E,1,8,0.95,39.9,M,17.8,M,,*63",
		&GPGGA{
			Time:           duration("6h49m51s"),
			UsedSatellites: 8,
			PositionFix:    1,
			Latitude:       23.11876,
			Longitude:      120.274063333333334,
			HDOP:           0.95,
			Altitude:       39.9}},

	// negative latitude and longitude.
	validSentence{
		"$GPGGA,064951.000,2307.1256,S,12016.4438,W,1,8,0.95,39.9,M,17.8,M,,*",
		&GPGGA{
			Time:           duration("6h49m51s"),
			UsedSatellites: 8,
			PositionFix:    1,
			Latitude:       -23.11876,
			Longitude:      -120.274063333333334,
			HDOP:           0.95,
			Altitude:       39.9}},

	//
	// GPRMC

	// before a fix.
	validSentence{
		"$GPRMC,064951.000,V,,,,,0.00,0.00,260406,,,N*",
		&GPRMC{
			Time:      time.Date(2006, 4, 26, 6, 49, 51, 0, time.UTC),
			Status:    'V',
			Latitude:  0,
			Longitude: 0,
			Mode:      'N',
			Speed:     0,
			Heading:   0}},

	// after a fix.
	validSentence{
		"$GPRMC,064951.000,A,2307.1256,N,12016.4438,E,0.03,165.48,260406,,,A*",
		&GPRMC{
			Time:      time.Date(2006, 4, 26, 6, 49, 51, 0, time.UTC),
			Status:    'A',
			Latitude:  23.11876,
			Longitude: 120.274063333333334,
			Mode:      'A',
			Speed:     0.03,
			Heading:   165.48}},

	// negative latitude and longitude.
	validSentence{
		"$GPRMC,064951.000,A,2307.1256,S,12016.4438,W,0.03,165.48,260406,,,A*",
		&GPRMC{
			Time:      time.Date(2006, 4, 26, 6, 49, 51, 0, time.UTC),
			Status:    'A',
			Latitude:  -23.11876,
			Longitude: -120.274063333333334,
			Mode:      'A',
			Speed:     0.03,
			Heading:   165.48}},

	//
	// GPGSA

	validSentence{
		"$GPGSA,A,3,03,04,01,32,22,28,11,,,,,,2.32,0.95,2.11*",
		&GPGSA{
			Mode1: 'A',
			Mode2: '3',
			SVs:   []byte{3, 4, 1, 32, 22, 28, 11},
			PDOP:  2.32,
			HDOP:  0.95,
			VDOP:  2.11}}}

var invalidSentences = []string{
	// length.
	"$T*",
	// checksum.
	"$GPGGA,064951.000,,,,,0,0,,,M,,M,,*48",
	// units.
	"$GPGGA,064951.000,,,,,0,0,,,K,,M,,*",
	// latitude indicator.
	"$GPGGA,064951.000,2307.1256,X,12016.4438,W,1,8,0.95,39.9,M,17.8,M,,*",
	// longitude indicator.
	"$GPGGA,064951.000,2307.1256,N,12016.4438,X,1,8,0.95,39.9,M,17.8,M,,*"}

func TestIsValidSentence(t *testing.T) {
	visitor := &visitor{}

	for _, v := range validSentences {
		sentence := v.sentence

		// compute the checksum if needed.
		if strings.HasSuffix(sentence, "*") {
			sentence += checksum(sentence)
		}

		if !isValidSentence(sentence) {
			t.Errorf("`%s` should be valid", sentence)
		}

		expected := v.expected

		actual, _ := visitor.visit(sentence)

		if !reflect.DeepEqual(expected, actual) {
			t.Errorf(
				"`%s` result expected to be `%v` but it's actually `%v`",
				sentence,
				expected,
				actual)
		}
	}
}

func TestIsInvalidSentence(t *testing.T) {
	visitor := &visitor{}

	for _, sentence := range invalidSentences {
		// compute the checksum if needed.
		if strings.HasSuffix(sentence, "*") {
			sentence += checksum(sentence)
		}

		actual, _ := visitor.visit(sentence)

		if actual != nil {
			t.Errorf("`%s` should not be valid. instead got `%v`", sentence, actual)
		}
	}
}
