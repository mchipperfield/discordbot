package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	LoginUrl  string = "https://kingshot-giftcode.centurygame.com/api/player"
	RedeemUrl string = "https://kingshot-giftcode.centurygame.com/api/gift_code"
	Key       string = "mN4!pQs6JrYwV9"
)

var ActiveCodes = []string{
	"JackKaoAndKS",
	"VIP777",
}

var ExpiredCodes []string

// EncodePayload encodes the data map into a JSON string with an MD5 signature
// This is the required format for the KingShot API.
func EncodePayload(data map[string]string) (string, error) {
	values := url.Values{}

	// 2. Data Encoding/Serialization (The Python equivalent of f"{key}={...}")
	for key, value := range data {
		values.Set(key, value)
	}

	// Signature Generation (MD5 Hashing)
	// Combine encoded data and secret: "{encoded_data}{secret}"
	dataToHash := values.Encode() + Key

	// Calculate MD5 hash
	hasher := md5.New()
	hasher.Write([]byte(dataToHash))

	// Convert the hash bytes to a hex string
	signature := hex.EncodeToString(hasher.Sum(nil))

	// Return the original data, with signature
	data["sign"] = signature

	payload, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

// ErrCode is a custom type to handle err_code being a number or a string.
type ErrCode string

// UnmarshalJSON implements the json.Unmarshaler interface.
func (e *ErrCode) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as a string.
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*e = ErrCode(s)
		return nil
	}

	// If it's not a string, try to unmarshal as a number.
	var i int
	if err := json.Unmarshal(data, &i); err == nil {
		*e = ErrCode(strconv.Itoa(i))
		return nil
	}

	return fmt.Errorf("err_code is not a string or a number: %s", data)
}

// LoginResponse represents the response from the login endpoint.
// ErrCode is a custom type to handle err_code being a number or a string.
type LoginResponse struct {
	Code    int     `json:"code"`
	Message string  `json:"msg"`
	Data    any     `json:"data"`
	ErrCode ErrCode `json:"err_code"`
}

func Login(fid string) (*LoginResponse, error) {
	data := map[string]string{
		"fid":  fid,
		"time": fmt.Sprintf("%d", time.Now().Unix()+10),
	}

	payload, err := EncodePayload(data)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(LoginUrl, "application/json", strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var loginResp LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return nil, err
	}
	return &loginResp, nil
}

type RedeemResponse struct {
	Code    int     `json:"code"`
	Message string  `json:"msg"`
	ErrCode ErrCode `json:"err_code"`
}

func RedeemGiftCode(fid, cdk string) (*RedeemResponse, error) {
	data := map[string]string{
		"fid":  fid,
		"cdk":  cdk,
		"time": fmt.Sprintf("%d", time.Now().Unix()+10),
	}

	payload, err := EncodePayload(data)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(RedeemUrl, "application/json", strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var redeemResponse RedeemResponse
	if err := json.NewDecoder(resp.Body).Decode(&redeemResponse); err != nil {
		return nil, err
	}

	return &redeemResponse, nil
}
