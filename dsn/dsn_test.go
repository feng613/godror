// Copyright 2017, 2020 The Godror Authors
//
//
// SPDX-License-Identifier: UPL-1.0 OR Apache-2.0

package dsn

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	errors "golang.org/x/xerrors"
)

func TestParse(t *testing.T) {
	t.Parallel()
	cc := DefaultConnectionClass
	if DefaultStandaloneConnection {
		cc = ""
	}
	wantAt := ConnectionParams{
		CommonParams: CommonParams{
			Username:      "cc",
			Password:      Password{"c@c*1"},
			ConnectString: "192.168.1.1/cc",
			Timezone:      time.Local,
		},
		PoolParams: PoolParams{
			MaxLifeTime:    DefaultMaxLifeTime,
			SessionTimeout: DefaultSessionTimeout,
			WaitTimeout:    DefaultWaitTimeout,
		},
	}
	wantDefault := ConnectionParams{
		StandaloneConnection: DefaultStandaloneConnection,
		CommonParams: CommonParams{
			Username:      "user",
			Password:      Password{"pass"},
			ConnectString: "sid",
			Timezone:      time.Local,
		},
		ConnParams: ConnParams{
			ConnClass: DefaultConnectionClass,
		},
		PoolParams: PoolParams{
			MinSessions:      DefaultPoolMinSessions,
			MaxSessions:      DefaultPoolMaxSessions,
			SessionIncrement: DefaultPoolIncrement,
			MaxLifeTime:      DefaultMaxLifeTime,
			SessionTimeout:   DefaultSessionTimeout,
			WaitTimeout:      DefaultWaitTimeout,
		},
	}
	wantDefault.ConnClass = DefaultConnectionClass
	if wantDefault.StandaloneConnection {
		wantDefault.ConnClass = ""
	}

	wantXO := wantDefault
	wantXO.ConnectString = "localhost/sid"

	wantHeterogeneous := wantXO
	wantHeterogeneous.Heterogeneous = true
	wantHeterogeneous.StandaloneConnection = false
	wantHeterogeneous.ConnClass = DefaultConnectionClass
	//wantHeterogeneous.PoolParams.Username, wantHeterogeneous.PoolParams.Password = "", ""

	cmpOpts := []cmp.Option{
		cmpopts.IgnoreUnexported(ConnectionParams{}),
		//cmp.Comparer(func(a, b ConnectionParams) bool {
		//return a.String() == b.String()
		//}),
		cmp.Comparer(func(a, b *time.Location) bool {
			if a == b {
				return true
			}
			now := time.Now()
			const tzFmt = "2006-01-02T15:04:05"
			return now.In(a).Format(tzFmt) == now.In(b).Format(tzFmt)
		}),
		cmp.Comparer(func(a, b Password) bool { return a.secret == b.secret }),
	}

	setP := func(s, p string) string {
		if i := strings.Index(s, ":SECRET-"); i >= 0 {
			if j := strings.Index(s[i:], "@"); j >= 0 {
				return s[:i+1] + p + s[i+j:]
			}
		} else if i := strings.Index(s, "=\"SECRET-"); i >= 0 {
			if j := strings.Index(s[i+2:], "\" "); j >= 0 {
				return s[:i+2] + p + s[i+2+j:]
			}
		} else if i := strings.Index(s, "=SECRET-"); i >= 0 {
			if j := strings.Index(s[i+1:], " "); j >= 0 {
				return s[:i+1] + p + s[i+1+j:]
			}
		}
		return s
	}
	// "scott@tcps://salesserver1:1521/sales.us.example.com?ssl_server_cert_dn=\"cn=sales,cn=Oracle Context Server,dc=us,dc=example,dc=com\"&sdu=8128&connect_timeout=60",
	wantEasy := wantDefault
	wantEasy.Username = "scott"
	wantEasy.Password.Reset()
	wantEasy.ConnectString = "tcps://salesserver1:1521/sales.us.example.com?ssl_server_cert_dn=\"cn=sales,cn=Oracle Context Server,dc=us,dc=example,dc=com\"&sdu=8128&connect_timeout=60"

	wantEmptyConnectString := wantDefault
	wantDefault.ConnectString = ""

	// From fuzzing
	for _, in := range []string{
		"oracle://[]",
		"@oracle://[]",
	} {
		if _, err := Parse(in); err != nil {
			t.Errorf("%q: %+v", in, err)
		}
	}
	for tName, tCase := range map[string]struct {
		In   string
		Want ConnectionParams
	}{
		"simple": {In: "user/pass@sid", Want: wantDefault},
		"userpass": {In:"user/pass", Want:wantEmptyConnectString},

		"full": {In: "oracle://user:pass@sid/?poolMinSessions=3&poolMaxSessions=9&poolIncrement=3&connectionClass=TestClassName&standaloneConnection=0&sysoper=1&sysdba=0&poolWaitTimeout=200ms&poolSessionMaxLifetime=4000s&poolSessionTimeout=2000s",
			Want: ConnectionParams{
				CommonParams: CommonParams{
					Username: "user", Password: Password{"pass"}, ConnectString: "sid",
					Timezone: time.Local,
				},
				ConnParams: ConnParams{
					ConnClass: "TestClassName", IsSysOper: true,
				},
				PoolParams: PoolParams{
					MinSessions: 3, MaxSessions: 9, SessionIncrement: 3,
					WaitTimeout: 200 * time.Millisecond, MaxLifeTime: 4000 * time.Second, SessionTimeout: 2000 * time.Second,
				},
			},
		},

		"@": {
			In:   setP(wantAt.String(), wantAt.Password.Secret()),
			Want: wantAt,
		},

		"xo":            {In: "oracle://user:pass@localhost/sid", Want: wantXO},
		"heterogeneous": {In: "oracle://user:pass@localhost/sid?heterogeneousPool=1", Want: wantHeterogeneous},

		"ipv6": {
			In: "oracle://[::1]:12345/dbname",
			Want: ConnectionParams{
				StandaloneConnection: DefaultStandaloneConnection,
				CommonParams: CommonParams{
					ConnectString: "[::1]:12345/dbname",
					Timezone:      time.Local,
				},
				ConnParams: ConnParams{
					ConnClass: cc,
				},
				PoolParams: PoolParams{
					MinSessions: 1, MaxSessions: 1000, SessionIncrement: 1,
					WaitTimeout: 30 * time.Second, MaxLifeTime: 1 * time.Hour, SessionTimeout: 5 * time.Minute,
					ExternalAuth: DefaultStandaloneConnection,
				},
			},
		},

		"easy": {
			In:   "scott@tcps://salesserver1:1521/sales.us.example.com?ssl_server_cert_dn=\"cn=sales,cn=Oracle Context Server,dc=us,dc=example,dc=com\"&sdu=8128&connect_timeout=60",
			Want: wantEasy,
		},

		"logfmt":           {In: "user=user password=pass connectString=localhost/sid heterogeneousPool=1", Want: wantHeterogeneous},
		"logfmt_oldpw":     {In: "connectString=user/pass@localhost/sid heterogeneousPool=1", Want: wantHeterogeneous},
		"logfmt_multiline": {In: "user=user\npassword=pass\nconnectString=localhost/sid\nheterogeneousPool=1", Want: wantHeterogeneous},

		"onInit": {In: "oracle://user:pass@sid/?poolMinSessions=3&poolMaxSessions=9&poolIncrement=3&connectionClass=TestClassName&standaloneConnection=0&sysoper=1&sysdba=0&poolWaitTimeout=200ms&poolSessionMaxLifetime=4000s&poolSessionTimeout=2000s&onInit=a&onInit=b",
			Want: ConnectionParams{
				CommonParams: CommonParams{
					Username: "user", Password: Password{"pass"}, ConnectString: "sid",
					Timezone:    time.Local,
					OnInitStmts: []string{"a", "b"},
				},
				ConnParams: ConnParams{
					ConnClass: "TestClassName", IsSysOper: true,
				},
				PoolParams: PoolParams{
					MinSessions: 3, MaxSessions: 9, SessionIncrement: 3,
					WaitTimeout: 200 * time.Millisecond, MaxLifeTime: 4000 * time.Second, SessionTimeout: 2000 * time.Second,
				},
			},
		},
	} {
		tCase := tCase
		t.Run(tName, func(t *testing.T) {
			t.Log(tCase.In)
			P, err := Parse(tCase.In)
			if err != nil {
				t.Errorf("%v", err)
				return
			}
			if diff := cmp.Diff(tCase.Want, P, cmpOpts...); diff != "" && tCase.Want.String() != P.String() {
				t.Errorf("parse of %q\ngot\n\t%#v,\nwanted\n\t%#v\n%s", tCase.In, P, tCase.Want, diff)
				return
			}
			s := setP(P.String(), P.Password.Secret())
			Q, err := Parse(s)
			if err != nil {
				t.Errorf("parseConnString %v", err)
				return
			}
			if diff := cmp.Diff(P, Q, cmpOpts...); diff != "" && P.String() != Q.String() {
				t.Errorf("params got %+v, wanted %+v\n%s", P, Q, diff)
				return
			}
			if got := setP(Q.String(), Q.Password.Secret()); s != got {
				t.Errorf("paramString got %q, wanted %q", got, s)
			}
		})
	}
}

func TestParseTZ(t *testing.T) {
	for k, v := range map[string]int{
		"00:00": 0, "+00:00": 0, "-00:00": 0,
		"01:00": 3600, "+01:00": 3600, "-01:01": -3660,
		"+02:00": 7200,
		"+02:03": 7380,
	} {
		i, err := ParseTZ(k)
		t.Logf("Parse(%q): %d %v", k, i, err)
		if err != nil {
			t.Fatal(errors.Errorf("%s: %w", k, err))
		}
		if i != v {
			t.Errorf("%s. got %d, wanted %d.", k, i, v)
		}
	}
}

func ExampleAppendLogfmt() {
	var buf strings.Builder
	AppendLogfmt(&buf, "user", "scott")
	AppendLogfmt(&buf, "password", "tiger")
	AppendLogfmt(&buf, "connectString", "dbhost:1521/orclpdb1?connect_timeout=2")
	fmt.Println(buf.String())
	// Output:
	// user=scott
	// password=tiger
	// connectString="dbhost:1521/orclpdb1?connect_timeout=2"
}

func ExampleConnectString() {
	var P ConnectionParams
	P.Username, P.Password = "scott", NewPassword("tiger")
	P.ConnectString = "dbhost:1521/orclpdb1?connect_timeout=2"
	P.SessionTimeout = 42 * time.Second
	P.SetSessionParamOnInit("NLS_NUMERIC_CHARACTERS", ",.")
	P.SetSessionParamOnInit("NLS_LANGUAGE", "FRENCH")
	fmt.Println(P.StringWithPassword())
	// Output:
	// user=scott password=tiger connectString="dbhost:1521/orclpdb1?connect_timeout=2"
	// alterSession="NLS_NUMERIC_CHARACTERS=,." alterSession="NLS_LANGUAGE=FRENCH"
	// configDir= connectionClass= enableEvents=0 heterogeneousPool=0 libDir= newPassword=
	// poolIncrement=0 poolMaxSessions=0 poolMinSessions=0 poolSessionMaxLifetime=0s
	// poolSessionTimeout=42s poolWaitTimeout=0s prelim=0 standaloneConnection=0
	// sysasm=0 sysdba=0 sysoper=0 timezone=local
}
