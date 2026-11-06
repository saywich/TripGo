package model

type IdempotencyRecord struct {
	RequestHash  []byte
	RequestBody  []byte
	Status       int
	ContentType  string
	Location     string
	ResponseBody []byte
}
