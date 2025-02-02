package blogging

type ClientConfig interface {
	LoadFromPersistableDict(map[string]string) error
	DumpToPersistableDict() map[string]string
}
