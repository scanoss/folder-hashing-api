#!/bin/bash
###
# SPDX-License-Identifier: GPL-2.0-or-later
#
# Copyright (C) 2018-2023 SCANOSS.COM
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 2 of the License, or
# (at your option) any later version.
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
# You should have received a copy of the GNU General Public License
# along with this program.  If not, see <https://www.gnu.org/licenses/>.
###

#!/bin/bash

# Simple mock for the ldb binary
read input

# Check if input matches the expected pattern: select from KB/TABLE key HEX
if [[ $input =~ key[[:space:]]+([[:xdigit:]]+)[[:space:]]+csv ]]; then
   key="${BASH_REMATCH[1]}"
   # If the key matches, return the predefined result
   if [[ "$key" == "00075de93df18ea6" ]]; then
       echo "00075de93df18ea6,271a83cbd2ce447978f5b2613706c2928758197999218c06921d82b96c09e04bd8734a0f3b56785a55fd79b53d10b8b369ac01a45083d17b3bf867db84b2696ce6a401106929958886d61a588dec8a9a02eb218a4d2f223b8049fcefc7c0819a89ead32e2406fb94af25809fa0fbd3c1c3571b742324d76e22c80ea3ca7d9d85e0586ab3d6845cf3676199bac77f8a94460601e2f81f1ddce57e67e04d22b3"
       exit 0
   fi
fi

# If input doesn't match or key is not the expected one, exit with error
exit 1