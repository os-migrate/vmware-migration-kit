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
package moduleutils

import "testing"

func TestFixedIPsForNeutron_IPv4AndIPv6Mixed(t *testing.T) {
	in := []string{"10.13.45.24", "fe80::123:56ff:1234:143a", "2001:db8::1"}
	got := FixedIPsForNeutron(in)
	if len(got) != 1 || got[0] != "10.13.45.24" {
		t.Errorf("expected [10.13.45.24], got %#v", got)
	}
}

func TestFixedIPsForNeutron_OnlyLinkLocalV6(t *testing.T) {
	in := []string{"fe80::1", "fe80::1234:5678:9abc:def0"}
	got := FixedIPsForNeutron(in)
	if len(got) != 0 {
		t.Errorf("expected empty, got %#v", got)
	}
}

func TestFixedIPsForNeutron_Empty(t *testing.T) {
	if got := FixedIPsForNeutron(nil); got != nil {
		t.Errorf("expected nil, got %#v", got)
	}
	if got := FixedIPsForNeutron([]string{}); got != nil {
		t.Errorf("expected nil, got %#v", got)
	}
}

func TestFixedIPsForNeutron_InvalidSkipped(t *testing.T) {
	in := []string{"not-an-ip", "10.0.0.1"}
	got := FixedIPsForNeutron(in)
	if len(got) != 1 || got[0] != "10.0.0.1" {
		t.Errorf("expected [10.0.0.1], got %#v", got)
	}
}
