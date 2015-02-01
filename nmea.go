// Developed by Rui Lopes (ruilopes.com). Released under the LGPLv3 license.

package nmea

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// NB When PositionFix=0 you should only use the Time and UsedSatellites fields.
type GPGGA struct {
	Time           time.Duration
	UsedSatellites byte
	PositionFix    byte // 0=Fix not available. 1=GPS fix. 2=Differential GPS fix.
	Latitude       float64
	Longitude      float64
	HDOP           float32
	Altitude       float32 // in meters.
}

type GPRMC struct {
	Time      time.Time
	Status    byte // A=data valid; V=data not valid.
	Latitude  float64
	Longitude float64
	Mode      byte    // A=Autonomous mode; D=Differential mode; E=Estimated mode. N=NULL.
	Speed     float32 // in knots.
	Heading   float32 // in degrees.
}

type GPGSA struct {
	Mode1 byte // M=Manual; A=Automatic
	Mode2 byte // 1=No fix; 2=2D (<4 used SVs); 3=3D (>=4 used SVs)
	SVs   []byte
	PDOP  float32
	HDOP  float32
	VDOP  float32
}

func isValidSentence(sentence string) bool {
	l := len(sentence)

	// the minimum length accepted sentence is $T,*CC.
	if l < 6 || sentence[0] != '$' || sentence[l-3] != '*' {
		return false
	}

	checksum := byte(0)

	for i := 1; i < l-3; i++ {
		checksum = checksum ^ byte(sentence[i])
	}

	expectedChecksumBytes, err := hex.DecodeString(sentence[l-2 : l])

	return err == nil && checksum == expectedChecksumBytes[0]
}

// Global Positioning System Fixed Data. Time, Position and fix.
//
// A non-fix example:
//
//	$GPGGA,064951.000,,,,,0,0,,,M,,M,,*4C
//
// A fix example:
//
// 	$GPGGA,064951.000,2307.1256,N,12016.4438,E,1,8,0.95,39.9,M,17.8,M,,*73
//
// Fields:
//
// +----+---------------+------------+--------+-------------------------------+
// |  # | name          | example    | units  | description                   |
// +----+---------------+------------+--------+-------------------------------+
// |  0 | UTC Time      | 064951.000 |        | hhmmss.sss                    |
// |  1 | Latitude      | 2307.1256  |        | ddmm.mmmm                     |
// |  2 | N/S Indicator | N          |        | N=north or S=south            |
// |  3 | Longitude     | 12016.4438 |        | dddmm.mmmm                    |
// |  4 | E/W Indicator | E          |        | E=east or W=west              |
// |  5 | Position Fix  | 1          |        | 0=Fix not available           |
// |    |               |            |        | 1=GPS fix                     |
// |    |               |            |        | 2=Differential GPS fix        |
// |  6 | Satellites    | 8          |        | Range 0 to 12                 |
// |    | Used          |            |        |                               |
// |  7 | HDOP          | 0.95       |        | Horizontal Dilution of        |
// |    |               |            |        | Precision                     |
// |  8 | MSL Altitude  | 39.9       | meters | Antenna Altitude above/below  |
// |    |               |            |        | mean-sea-level                |
// |  9 | Units         | M          | meters | Units of antenna altitude     |
// | 10 | Geoidal       | 17.8       | meters |                               |
// |    | Separation    |            |        |                               |
// | 11 | Units         | M          | meters | Units of geoids separation    |
// | 12 | Age of Diff.  |            | second | Null fields when DGPS is not  |
// |    | Corr.         |            |        | used                          |
// | 13 | unknown       |            |        |                               |
// +----+---------------+------------+--------+-------------------------------+
func parseGPGGA(sentence string) (*GPGGA, error) {
	result := &GPGGA{}

	fields := splitFields(sentence)

	if len(fields) != 14 {
		return nil, fmt.Errorf("Failed to parse GPGGA. invalid number of fields %v", len(fields))
	}

	//
	// time. e.g.: 064951.000 format: hhmmss.sss
	timeMs, err := parseTime(fields[0])
	if err != nil {
		return nil, err
	}

	result.Time = time.Duration(timeMs) * time.Millisecond

	//
	// latitude.  if available. e.g.: 2307.1256  format: ddmm.mmmm
	// longitude. if available. e.g.: 12016.4438 format: dddmm.mmmm
	latitudeField := fields[1]
	longitudeField := fields[3]

	if len(latitudeField) > 0 && len(longitudeField) > 0 {
		latitude, err := parseLatitude(latitudeField, fields[2])
		if err != nil {
			return nil, err
		}

		longitude, err := parseLongitude(longitudeField, fields[4])
		if err != nil {
			return nil, err
		}

		result.Latitude = latitude
		result.Longitude = longitude
	}

	positionFix, err := strconv.ParseInt(fields[5], 10, 8)
	if err != nil {
		return nil, err
	}
	result.PositionFix = byte(positionFix)

	usedSatellites, err := strconv.ParseInt(fields[6], 10, 8)
	if err != nil {
		return nil, err
	}
	result.UsedSatellites = byte(usedSatellites)

	hdopField := fields[7]
	if len(hdopField) > 0 {
		hdop, err := strconv.ParseFloat(hdopField, 32)
		if err != nil {
			return nil, err
		}
		result.HDOP = float32(hdop)
	}

	altitudeField := fields[8]
	if len(altitudeField) > 0 {
		altitude, err := strconv.ParseFloat(altitudeField, 32)
		if err != nil {
			return nil, err
		}
		result.Altitude = float32(altitude)
	}

	if fields[9] != "M" {
		return nil, fmt.Errorf("Altitude unit not supported: %s", fields[9])
	}

	return result, nil
}

// Global Positioning Recommended Minimum Navigation Information.
//
// Example without a fix:
//
// 	$GPRMC,064951.000,V,,,,,0.00,0.00,260406,,,N*
//
// Example with a fix:
//
// 	$GPRMC,064951.000,A,2307.1256,N,12016.4438,E,0.03,165.48,260406,,,A*
//
// Fields
// +----+---------------+------------+---------+------------------------------+
// |  # | name          | example    | units   | description                  |
// +----+---------------+------------+---------+------------------------------+
// |  0 | UTC Time      | 064951.000 |         | hhmmss.sss                   |
// |  1 | Status        | A          |         | A=data valid                 |
// |    |               |            |         | V=data not valid             |
// |  2 | Latitude      | 2307.1256  |         | ddmm.mmmm                    |
// |  3 | N/S Indicator | N          |         | N=north or S=south           |
// |  4 | Longitude     | 12016.4438 |         | dddmm.mmmm                   |
// |  5 | E/W Indicator | E          |         | E=east or W=west             |
// |  6 | Speed over    | 0.03       | knots   |                              |
// |    | Groud         |            |         |                              |
// |  7 | Course over   | 165.48     | degrees |                              |
// |    | Groud         |            |         |                              |
// |  8 | Date          | 260406     |         | ddmmyy                       |
// |  9 | Magnetic      | 3.05       | degrees | Needs GlobalTop              |
// |    | Variation     |            |         | Customization Service        |
// | 10 | Magnetic      | W          |         | E=east or W=west (Needs      |
// |    | Variation     |            |         | GlobalTop Customization      |
// |    | E/W indicator |            |         | Service)                     |
// | 11 | Mode          | A          |         | A=Autonomous mode            |
// |    |               |            |         | D=Differential mode          |
// |    |               |            |         | E=Estimated mode             |
// |    |               |            |         | N=NULL (I didn't see this on |
// |    |               |            |         |   the datasheet, but on a    |
// |    |               |            |         |   real device)               |
// +----+---------------+------------+---------+------------------------------+
func parseGPRMC(sentence string) (*GPRMC, error) {
	result := &GPRMC{}

	fields := splitFields(sentence)

	if len(fields) != 12 {
		return nil, fmt.Errorf("Failed to parse GPRMC. invalid number of fields %v", len(fields))
	}

	//
	// time. e.g.: 064951.000 format: hhmmss.sss
	timeMs, err := parseTime(fields[0])
	if err != nil {
		return nil, err
	}

	//
	// date. e.g.: 260406 format: ddmmyy
	date, err := parseDate(fields[8])
	if err != nil {
		return nil, err
	}

	result.Time = date.Add(time.Duration(timeMs) * time.Millisecond)

	//
	// status.
	if len(fields[1]) != 1 {
		return nil, fmt.Errorf("Failed to parse status %s", fields[1])
	}

	status := byte(fields[1][0])

	if status != 'A' && status != 'V' {
		return nil, fmt.Errorf("Failed to parse status %s", status)
	}

	result.Status = status

	//
	// latitude.  if available. e.g.: 2307.1256  format: ddmm.mmmm
	// longitude. if available. e.g.: 12016.4438 format: dddmm.mmmm
	latitudeField := fields[2]
	longitudeField := fields[4]

	if len(latitudeField) > 0 && len(longitudeField) > 0 {
		latitude, err := parseLatitude(latitudeField, fields[3])
		if err != nil {
			return nil, err
		}

		longitude, err := parseLongitude(longitudeField, fields[5])
		if err != nil {
			return nil, err
		}

		result.Latitude = latitude
		result.Longitude = longitude
	}

	//
	// speed.
	speed, err := strconv.ParseFloat(fields[6], 32)
	if err != nil {
		return nil, err
	}
	result.Speed = float32(speed)

	//
	// heading.
	heading, err := strconv.ParseFloat(fields[7], 32)
	if err != nil {
		return nil, err
	}
	result.Heading = float32(heading)

	//
	// mode.
	if len(fields[11]) != 1 {
		return nil, fmt.Errorf("Failed to parse mode %v", fields[11])
	}

	mode := byte(fields[11][0])

	if mode != 'A' && mode != 'D' && mode != 'E' && mode != 'N' {
		return nil, fmt.Errorf("Failed to parse mode %v", mode)
	}

	result.Mode = mode

	return result, nil
}

// Global Positioning GNSS DOP and Active Satellites
//
// Example:
//
//	$GPGSA,A,3,03,04,01,32,22,28,11,,,,,,2.32,0.95,2.11*
//
// Fields:
//
// +----+--------+---------+----------------------------------+
// |  # | name   | example | description                      |
// +----+--------+---------+----------------------------------+
// |  0 | Mode 1 | A       | M=Manual                         |
// |    |        |         | A=Automatic                      |
// |  1 | Mode 2 | 3       | 1=No fix                         |
// |    |        |         | 2=2D (<4 used SVs)               |
// |    |        |         | 3=3D (>=4 used SVs)              |
// |  2 | SV 1   | 03      | SV on channel 1                  |
// |  3 | SV 2   | 04      | SV on channel 2                  |
// | .. | ...    |         | ...                              |
// | 13 | SV 12  |         | SV on channel 12                 |
// | 14 | PDOP   | 2.32    | Position Dilution of Precision   |
// | 15 | HDOP   | 0.95    | Horizontal Dilution of Precision |
// | 16 | VDOP   | 2.11    | Vertical Dilution of Precision   |
// +----+--------+---------+----------------------------------+
func parseGPGSA(sentence string) (*GPGSA, error) {
	result := &GPGSA{}

	fields := splitFields(sentence)

	if len(fields) != 17 {
		return nil, fmt.Errorf("Failed to parse GPGSA. invalid number of fields %v", len(fields))
	}

	//
	// mode 1.
	if len(fields[0]) != 1 {
		return nil, fmt.Errorf("Failed to parse mode1 %s", fields[0])
	}
	mode1 := byte(fields[0][0])
	if mode1 != 'M' && mode1 != 'A' {
		return nil, fmt.Errorf("Failed to parse mode1 %s", mode1)
	}
	result.Mode1 = mode1

	//
	// mode 2.
	if len(fields[1]) != 1 {
		return nil, fmt.Errorf("Failed to parse mode2 %s", fields[1])
	}
	mode2 := byte(fields[1][0])
	if mode2 != '1' && mode2 != '2' && mode2 != '3' {
		return nil, fmt.Errorf("Failed to parse mode2 %s", mode2)
	}
	result.Mode2 = mode2

	//
	// SVs.
	usedSVs := 0
	for i := 2; i < 12; i++ {
		if len(fields[i]) == 0 {
			break
		}
		usedSVs++
	}
	svs := make([]byte, usedSVs)
	for i := 0; i < usedSVs; i++ {
		svField := fields[2+i]
		sv, err := strconv.ParseInt(svField, 10, 8)
		if err != nil {
			return nil, fmt.Errorf("Failed to parse SV Channel %v %s", i, svField)
		}
		svs[i] = byte(sv)
	}
	result.SVs = svs

	// the xDOP fields are only available when there is a fix.
	if result.Mode2 != '1' {
		//
		// PDOP.
		pdop, err := strconv.ParseFloat(fields[14], 32)
		if err != nil {
			return nil, err
		}
		result.PDOP = float32(pdop)

		//
		// HDOP.
		hdop, err := strconv.ParseFloat(fields[15], 32)
		if err != nil {
			return nil, err
		}
		result.HDOP = float32(hdop)

		//
		// VDOP.
		vdop, err := strconv.ParseFloat(fields[16], 32)
		if err != nil {
			return nil, err
		}
		result.VDOP = float32(vdop)
	}

	return result, nil
}

// latitude. format: ddmm.mmmm e.g.: input: 2307.1256 output: 23.11876
// indicator. e.g.: N
func parseLatitude(text string, indicator string) (float64, error) {
	if len(text) != 9 || text[4] != '.' {
		return 0, fmt.Errorf("Failed to parse latitude %s", text)
	}

	if indicator != "N" && indicator != "S" {
		return 0, fmt.Errorf("Failed to parse latitude indicator %s", indicator)
	}

	degrees, err := strconv.ParseFloat(text[0:2], 64)
	if err != nil {
		return 0, fmt.Errorf("Failed to parse latitude degrees %s: %v", text, err)
	}

	minutes, err := strconv.ParseFloat(text[2:], 64)
	if err != nil {
		return 0, fmt.Errorf("Failed to parse latitude minutes %s: %v", text, err)
	}

	latitude := degrees + minutes/60

	if indicator == "S" {
		latitude *= -1
	}

	return latitude, nil
}

// longitude. format: dddmm.mmmm e.g.: input: 12016.4438 output: 120.274063333333334
// indicator. e.g.: E
func parseLongitude(text string, indicator string) (float64, error) {
	if len(text) != 10 || text[5] != '.' {
		return 0, fmt.Errorf("Failed to parse longitude %s", text)
	}

	if indicator != "E" && indicator != "W" {
		return 0, fmt.Errorf("Failed to parse longitude indicator %s", indicator)
	}

	degrees, err := strconv.ParseFloat(text[0:3], 64)
	if err != nil {
		return 0, fmt.Errorf("Failed to parse longitude degrees %s: %v", text, err)
	}

	minutes, err := strconv.ParseFloat(text[3:], 64)
	if err != nil {
		return 0, fmt.Errorf("Failed to parse longitude minutes %s: %v", text, err)
	}

	longitude := degrees + minutes/60

	if indicator == "W" {
		longitude *= -1
	}

	return longitude, nil
}

// parse time. format: hhmmss.sss e.g. 064951.000
func parseTime(text string) (int32, error) {
	if len(text) != 10 {
		return 0, fmt.Errorf("Failed to parse time %s: len is not 10", text)
	}

	h, err := strconv.ParseInt(text[0:2], 10, 8)
	if err != nil {
		return 0, fmt.Errorf("Failed to parse time %s: hour could not be parsed due to %v", text, err)
	}

	m, err := strconv.ParseInt(text[2:4], 10, 8)
	if err != nil {
		return 0, fmt.Errorf("Failed to parse time %s: minute could not be parsed due to %v", text, err)
	}

	s, err := strconv.ParseInt(text[4:6], 10, 8)
	if err != nil {
		return 0, fmt.Errorf("Failed to parse time %s: minute could not be parsed due to %v", text, err)
	}

	ms, err := strconv.ParseInt(text[7:10], 10, 16)
	if err != nil {
		return 0, fmt.Errorf("Failed to parse time %s: milliseconds could not be parsed due to %v", text, err)
	}

	return int32(ms) + int32(s)*1000 + int32(m)*1000*60 + int32(h)*1000*60*60, nil
}

// parse date. e.g.: 260406 format: ddmmyy
func parseDate(text string) (time.Time, error) {
	if len(text) != 6 {
		return time.Time{}, fmt.Errorf("Failed to parse date %s: len is not 6", text)
	}

	d, err := strconv.ParseInt(text[0:2], 10, 8)
	if err != nil {
		return time.Time{}, fmt.Errorf("Failed to parse date %s: day could not be parsed due to %v", text, err)
	}

	m, err := strconv.ParseInt(text[2:4], 10, 8)
	if err != nil {
		return time.Time{}, fmt.Errorf("Failed to parse date %s: month could not be parsed due to %v", text, err)
	}

	y, err := strconv.ParseInt(text[4:6], 10, 8)
	if err != nil {
		return time.Time{}, fmt.Errorf("Failed to parse date %s: year could not be parsed due to %v", text, err)
	}

	return time.Date(2000+int(y), time.Month(m), int(d), 0, 0, 0, 0, time.UTC), nil
}

func splitFields(sentence string) []string {
	fields := strings.Split(sentence, ",")

	l := len(fields)

	if l <= 1 {
		return make([]string, 0)
	}

	// remove the checksum from the last item, that is, the three last characters, e.g. *CC.
	lastField := fields[l-1]
	fields[l-1] = lastField[:len(lastField)-3]

	// skip the first item that contains the type. e.g. $GPGGA.
	return fields[1:]
}

type Visitor interface {
	OnBeforeParse(sentenceType, sentence string) bool
	OnAfterParse(sentenceType, sentence string, err error)
	OnGPGGA(sentence *GPGGA)
	OnGPRMC(sentence *GPRMC)
	OnGPGSA(sentence *GPGSA)
}

func Visit(reader io.Reader, visitor Visitor) error {
	scanner := bufio.NewScanner(reader)

	for scanner.Scan() {
		sentence := scanner.Text()

		if !isValidSentence(sentence) {
			continue
		}

		endOfTypeIndex := strings.IndexByte(sentence, ',')

		sentenceType := sentence[1:endOfTypeIndex]

		if !visitor.OnBeforeParse(sentenceType, sentence) {
			continue
		}

		var err error

		switch sentenceType {
		case "GPGGA":
			gpgga, err := parseGPGGA(sentence)

			if err == nil {
				visitor.OnGPGGA(gpgga)
			}

		case "GPRMC":
			gprmc, err := parseGPRMC(sentence)

			if err == nil {
				visitor.OnGPRMC(gprmc)
			}

		case "GPGSA":
			gpgsa, err := parseGPGSA(sentence)

			if err == nil {
				visitor.OnGPGSA(gpgsa)
			}

		// TODO GPVTG
		// TODO GPGSV

		default:
			err = nil // TODO use a UnknownSentenceError
		}

		visitor.OnAfterParse(sentenceType, sentence, err)
	}

	// TODO handle graceful shutdown of the scanner source. that is, when we close the source, return nil

	return scanner.Err()
}
