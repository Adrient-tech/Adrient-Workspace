package cloudimport

import (
	"strings"
	"testing"
)

const takeoutMbox = `From 1780000000000000001@xxx Mon Jan 02 10:00:00 +0000 2023
X-GM-THRID: 1
X-Gmail-Labels: Inbox,Unread,Clients/Acme,Category Updates
From: Bob <bob@acme.example>
To: ann@acme.example
Subject: Quarterly numbers
Message-ID: <m1@acme>
Date: Mon, 02 Jan 2023 10:00:00 +0000

Line one
>From the desk of Bob

From 1780000000000000002@xxx Mon Jan 02 11:00:00 +0000 2023
X-Gmail-Labels: Sent
From: ann@acme.example
To: carol@client.example
Subject: Re: proposal
Message-ID: <m2@acme>

Thanks
`

func TestSplitMboxAndTakeoutLabels(t *testing.T) {
	var msgs [][]byte
	if err := SplitMbox(strings.NewReader(takeoutMbox), func(b []byte) error { msgs = append(msgs, b); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages", len(msgs))
	}
	if !strings.Contains(string(msgs[0]), "\nFrom the desk of Bob") || strings.Contains(string(msgs[0]), ">From the desk") {
		t.Errorf("mboxrd unquoting failed: %q", msgs[0])
	}
	f, l := takeoutFolder(mboxGmailLabels(msgs[0]))
	if f != "INBOX" || len(l) != 1 || l[0] != "Clients/Acme" {
		t.Errorf("labels: %s %v", f, l)
	}
	mf, err := ParseMail(RemoteMail{Raw: msgs[1], Folder: "Sent"})
	if err != nil || mf.Folder != "sent" || mf.Subject != "Re: proposal" || mf.To[0] != "carol@client.example" {
		t.Errorf("parse: %+v %v", mf, err)
	}
}
