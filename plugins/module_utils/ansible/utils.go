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

package ansible

import (
	"encoding/json"
	"fmt"
	"os"
)

type Response struct {
	Msg     string   `json:"msg"`
	Changed bool     `json:"changed"`
	Failed  bool     `json:"failed"`
	ID      []string `json:"id"`
}

type MigrateResponse struct {
	Msg     string `json:"msg"`
	Changed bool   `json:"changed"`
	Failed  bool   `json:"failed"`
	Disks   []Disk `json:"disks"`
}

type Disk struct {
	ID      string `json:"id"`
	Primary bool   `json:"primary"`
}

func ExitJsonWithDeps(responseBody Response, exitFunc func(int), printFunc func(string)) {
    ReturnResponseWithDeps(responseBody, exitFunc, printFunc)
}

func ExitJson(responseBody Response) {
    ExitJsonWithDeps(responseBody, os.Exit, func(s string) { fmt.Println(s) })
}

func FailJsonWithDeps(responseBody Response, exitFunc func(int), printFunc func(string)) {
    responseBody.Failed = true
    ReturnResponseWithDeps(responseBody, exitFunc, printFunc)
}

func FailJson(responseBody Response) {
    FailJsonWithDeps(responseBody, os.Exit, func(s string) { fmt.Println(s) })
}

func RequireFieldWithDeps(field, errorMessage string, failHandler func(string)) string {
    if field == "" {
        failHandler(errorMessage)
    }
    return field
}

func RequireField(field, errorMessage string) string {
    return RequireFieldWithDeps(field, errorMessage, FailWithMessage)
}

func DefaultIfEmpty(field, defaultValue string) string {
	if field == "" {
		return defaultValue
	}
	return field
}

func FailWithMessageWithDeps(msg string, exitFunc func(int), printFunc func(string)) {
    response := Response{Msg: msg}
    FailJsonWithDeps(response, exitFunc, printFunc)
}

func FailWithMessage(msg string) {
    FailWithMessageWithDeps(msg, os.Exit, func(s string) { fmt.Println(s) })
}

func ReturnResponseWithDeps(
    responseBody Response,
    exitFunc func(int),
    printFunc func(string),
) {
    var response []byte
    var err error
    response, err = json.Marshal(responseBody)
    if err != nil {
        response, _ = json.Marshal(Response{Msg: "Invalid response object"})
    }
    printFunc(string(response))
    if responseBody.Failed {
        exitFunc(1)
    } else {
        exitFunc(0)
    }
}

// TODO: commented since it's not used
/*
func returnMResponse(responseBody Response) []Disk {
	var disks []Disk
	for _, id := range responseBody.ID {
		disks = append(disks, Disk{ID: id, Primary: true})
	}
	return disks
}
*/
