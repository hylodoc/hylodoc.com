package size

import "fmt"

type Size uint64

const (
	Byte     = 1
	Kilobyte = 1e3
	Megabyte = 1e6
	Gigabyte = 1e9
	Terabyte = 1e12
)

func (size Size) Abbrev(decimals int) string {
	switch {
	case size >= Terabyte:
		return fmt.Sprintf("%.*fTB", decimals, float64(size)/Terabyte)
	case size >= Gigabyte:
		return fmt.Sprintf("%.*fGB", decimals, float64(size)/Gigabyte)
	case size >= Megabyte:
		return fmt.Sprintf("%.*fMB", decimals, float64(size)/Megabyte)
	case size >= Kilobyte:
		/*
		 * The internationally recommended unit symbol for the kilobyte
		 * is _kB_.
		 *
		 * https://en.wikipedia.org/wiki/Kilobyte
		 */
		return fmt.Sprintf("%.*fkB", decimals, float64(size)/Kilobyte)
	default:
		return fmt.Sprintf("%.*f", decimals, float64(size))
	}
}

func (size Size) String() string {
	return size.Abbrev(2)
}
