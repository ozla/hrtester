package config

////////////////////////////////////////////////////////////////////////////////

const (
	DefaultPort = 51250
)

////////////////////////////////////////////////////////////////////////////////

var (
	Tester = struct {
		Collector     string
		Target        string
		CAs           string
		Cert          string
		Key           string
		SkipNameCheck bool
		Port          uint16
	}{}

	Collector = struct {
		CSVFile string
		Port    uint16
	}{}

	Mocker = struct {
		CAs  string
		Cert string
		Key  string
		Port uint16
	}{}
)

////////////////////////////////////////////////////////////////////////////////
