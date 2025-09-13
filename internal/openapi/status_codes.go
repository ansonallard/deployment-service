package openapi

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

const (
	baseTen                  = 10
	thirtyTwoBitsOfPrecision = 32
)

func GetFirstSuccessfulStatusCode(responses *openapi3.Responses) (int, error) {
	httpResponseCodes := []int{}
	for responseCode := range responses.Map() {
		// Skip catchall status codes
		if strings.Contains(responseCode, "X") {
			continue
		}
		intResponseCode, err := strconv.ParseInt(responseCode, baseTen, thirtyTwoBitsOfPrecision)
		if err != nil {
			return 0, fmt.Errorf("someone messed up here - we should be able to parse http status codes")
		}
		httpResponseCodes = append(httpResponseCodes, int(intResponseCode))
	}
	sort.Slice(httpResponseCodes, func(i, j int) bool {
		return httpResponseCodes[i] < httpResponseCodes[j]
	})

	if len(httpResponseCodes) == 0 {
		return 0, fmt.Errorf("no valid status codes present in openapi responses")
	}
	firstSuccessfulStatusCode := httpResponseCodes[0]
	if firstSuccessfulStatusCode >= http.StatusBadRequest {
		return 0, fmt.Errorf("no successful status codes present")
	}
	return firstSuccessfulStatusCode, nil
}
