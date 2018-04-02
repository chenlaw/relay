/*

  Copyright 2017 Loopring Project Ltd (Loopring Foundation).

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.

*/

package main

import "gopkg.in/urfave/cli.v1"

func chainclientCommands() cli.Command {
	c := cli.Command{
		Name:        "chainclient",
		Usage:       "chainclient ",
		Category:    "Chainclient Commands",
		Subcommands: []cli.Command{
		//秘钥以及地址，生成时的密码
		},
	}
	return c
}