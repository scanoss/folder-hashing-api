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
   if [[ "$key" == "6a00168a94ae6238" ]]; then
       echo "6a00168a94ae6238,6b0a6c14734147e0,0134f476e2548425,"
       exit 0
   fi
fi

# If input doesn't match or key is not the expected one, exit with error
exit 1