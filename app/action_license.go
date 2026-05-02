package app

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

// licenseCommand registers the `znnd license` sub-command, which
// prints the GPL-v3 license summary.
var (
	licenseCommand = &cli.Command{
		Action:    licenseAction,
		Name:      "license",
		Usage:     "Display license information",
		ArgsUsage: " ",
		Category:  "MISCELLANEOUS COMMANDS",
	}
)

// licenseAction is the handler for `znnd license` — prints the
// GPL-v3 summary block to stdout.
func licenseAction(*cli.Context) error {
	fmt.Println(`znnd is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

znnd is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU General Public License for more details.

You should have received chain copy of the GNU General Public License
along with znnd. If not, see <http://www.gnu.org/licenses/>.`)
	return nil
}
