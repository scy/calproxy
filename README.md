# calproxy

This is a simple tool that fetches a iCal file in a regular interval. 
The calendar is made available in a "private version" that contains the original file and a "free/busy version" that provides redacted events (no title, description, attendants etc.). 
Both versions can be accessed by two different secret URLs that don't require authentication.

## Use Case

The groupware tool we use at work only supports three modes of sharing my personal calendar:

* full access in iCal format if I supply my username and password
* sharing to other users in the same system
* a public sharing option where everyone can read my calendar if they know my username

However, what I want is to achieve two things:

* have my calendar in my Mac's calendar app and in my personal Google calendar _without_ telling them my groupware password
* let my girlfriend see when my meetings are without automatically telling her what they are about (think company NDAs, not trust issues)

Both aren't really achievable with what the groupware provides, but if I had a tool that used the first option above (full access with credentials) to generate a personal and a redacted version of the calendar…

That's exactly what calproxy does.

## Status

It's working. 
The code is ugly, but should be secure.

Also, this is something that I built for myself, and I won't hesitate to radically change things if I need to. 
Therefore, to be explicit: 
**I don't promise API stability, bug fixes or integratability.** 
The last one isn't even a word! 
If you use this and it works for you, fine. 
If I change something and it breaks your setup, tough luck.

## Building

calproxy is written in Go. 
Use `go build src/calproxy.go` or something like that to compile it into multiple megabytes of binary awesomeness.

## Usage

This thing is totally stateless. 
It's configured using environment variables only. 
It doesn't read or write any files, the calendars are kept in memory.

These are the variables you should provide:

* **CALPROXY_PORT:** 
  The port calproxy should listen on for incoming HTTP requests. 
  It doesn't do authentication, you can't specify an interface.
* **CALPROXY_ORIGIN:** 
  The URL of the calendar you want to mirror. 
  If it requires a username and password, provide it in the URL the usual way, i.e. `https://user:password@example.com/my-little-calendar.ics`. 
  If you do not provide this variable, you will be prompted to enter the origin on stdin. 
  Local terminal echo will be disabled then. 
  This allows you to enter (or paste) the URL without anyone seeing it and without it being accessible in the environment variables of the process.
* **CALPROXY_SECRET:** 
  Set this to a random string. 
  The "private version" of your calendar will have a file name that's `sha512(secret + sha512(origin_url))`. 
  This means that if you want to invalidate the private URL, you can set this value to something else. 
  It also means that if you _don't_ want to invalidate the URL, you should keep this constant. 
  Don't change it every time you launch calproxy!
* **CALPROXY_UPDATE_SECS:** 
  Set this to a (positive integer) number of seconds to specify how often you would like to fetch the origin. 
  If you don't specify it, a default of 16 minutes will be used.
* **CALPROXY_FB_TITLE:** 
  The title (or "summary") the events in the free/busy version should have. 
  If you don't provide this, they won't have one. 
  If you provide one, you shouldn't use fancy characters like quotes, colons or linebreaks, since it don't escape shit. 
  Something like `(busy)` is probably a good idea.

After starting up, it you will find the path names of the private and free/busy calendars in the log that's written to stderr. 
Free/busy will be at `sha512(origin_url) + ".ics"`, the private version at `sha512(secret + sha512(origin_url)) + ".ics"`.

## Security

* As long as SHA512 is secure, it shouldn't be possible to guess the original URL (containing username and password) in a reasonable amount of time.
* If you fetch the origin via an unencrypted connection (HTTP instead of HTTPS), your credentials and calendar contents can be read via MitM attacks.
* If your clients connect to calproxy via an unencrypted connection, MitM attackers can't see your origin credentials, but they can still see the contents of the feed you requested (private or free/busy) as well as the URL you were using. 
  To put it another way: 
  **If your origin is on HTTPS, but your connection to calproxy isn't, you are happily leaking your calendar contents and made things worse.**

## Bugs

* I have tested with with the iCal returned by our groupware only. 
  It could refuse to work with other inputs; the iCal format is crap.
* It should probably provide some additional HTTP headers, for example a last modified timestamp or a nerdy pop culture reference.
* It doesn't provide real free/busy data, i.e. no `VFREEBUSY` blocks. 
  That's because these cannot have `RRULE` repetition rule definitions, which means that I would have to convert the RRULEs to multiple time intervals myself. 
  I am **so** not doing that. 
  Instead, I am providing redacted `VEVENT` blocks.
* It should do something sensible when it couldn't fetch the origin for a certain time, e.g. crash.
* When the initial fetch or parsing fail, calproxy will die with an error message. 
  However, when one of the scheduled updates does not succeed (no matter whether fetching or parsing failed), the error will be silently ignored. 
  The idea behind this is that the error might just be temporary; however, this isn't optimal yet. 
* Since it fetches the origin calendar every _n_ seconds, regardless of whether there are incoming HTTP requests or not, one might argue whether it's a proxy at all. 
  Maybe I'll rename it to _calcronjob-with-http_ just to annoy people.

## Meta

calproxy was written by Tim Weber (aka scy) in March 2018. 
Its official home on the web is at `https://github.com/scy/calproxy`.
