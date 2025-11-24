package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

const (
	LoginUrl  string = "https://kingshot-giftcode.centurygame.com/api/player"
	RedeemUrl string = "https://kingshot-giftcode.centurygame.com/api/gift_code"
	Key       string = "mN4!pQs6JrYwV9"
)

var ActiveCodes = []string{
	"JackKaoAndKS",
	"VIP777",
	"invalidcode123",
	"TRICKORTREAT",
}

var ExpiredCodes []string

type LoginRequest struct {
	Fid  string `json:"fid"`
	Time string `json:"time"`
	Sign string `json:"sign"`
}

func EncodePayload(data map[string]string) string {

	var sortedKeys []string
	for key := range data {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)

	// 2. Data Encoding/Serialization (The Python equivalent of f"{key}={...}")
	var encodedPairs []string
	for _, key := range sortedKeys {
		value := data[key]
		valueStr := fmt.Sprint(value)

		// Append the key=value pair
		encodedPairs = append(encodedPairs, fmt.Sprintf("%s=%s", key, valueStr))
	}

	// Join the pairs with '&'
	encodedData := strings.Join(encodedPairs, "&")

	// 3. Signature Generation (MD5 Hashing)
	// Combine encoded data and secret: "{encoded_data}{secret}"
	dataToHash := encodedData + Key

	// Calculate MD5 hash
	hasher := md5.New()
	hasher.Write([]byte(dataToHash))

	// Convert the hash bytes to a hex string
	signature := hex.EncodeToString(hasher.Sum(nil))

	// 4. Return the new map with the signature
	// Create a new map and copy all original data, then add the 'sign' field
	signedData := make(map[string]interface{})
	for k, v := range data {
		signedData[k] = v
	}
	signedData["sign"] = signature

	payload, err := json.Marshal(signedData)
	if err != nil {
		panic(err)
	}
	return string(payload)
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

func Login(payload string) (*LoginResponse, error) {
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

func RedeemGiftCode(payload string) (*RedeemResponse, error) {
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
