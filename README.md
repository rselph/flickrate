# flickrate

flickrate is a different way to look at the statistics for your flickr photos.
On their statistics page, flickr tracks the top ten photos in terms of all time cumulative
views.  But after a while, this list rarely changes, because older photos keep accumulating
views and newer ones can never catch up.  I was interested in which of my more recent
photos were doing well, so I wrote flickrate.  This program will show you which of your
photos are getting more views per day, averaged over the time since they were
posted. This should allow recent photos a chance, but still allow the all-time
blockusters to show up.

flickrate is a command line application.  If you're not comfortable with the
command line, I'm afraid you're not going to find it terribly easy to use.  If
the shell prompt doesn't intimidate you, then read on!

## First Steps
To use flickrate, you'll need an API key and secret from flickr.  You can get them
[here](https://www.flickr.com/services/apps/create/noncommercial/?).  The first time
you use it, you'll have to supply these to flickrate using the `-key` and `-secret`
flags.  After that, flickrate will remember them, so you don't have to
type them every time.  If you want to check photos that are private, or that
are not visible in safe mode, you'll need to authorize flickrate.  Do this by
providing the `-user` flag with your user name.  Again, flickrate will remember the login,
so you don't have to do it every time.

## Usage
Once all of that red tape is out of the way, simply run flickrate with the
name of the user whose stats you want to check.  If you've authorized
flickrate with the `-user` parameter, then by default your own photos
will be checked.  Use the `-h` or `--help` flags to see what all of the
options are.  The most useful ones are:

* `-mindays` sets the minimum age for photos to be considered.  Very
recently posted photos will have a much higher average view rate, so they will
distort the results.  By default flickrate will only consider photos at
least 60 days old.  Use this parameter to change that.
* `-maxdays` sets the maximum age for photos to be considered.  If you're
only interested in photos posted within the last year, for instance, you
could provide the value 365.  By default, flickrate does not impose a
maximum age.
* `-minviews` sets the minimum number of views a photo must have to be
considered.  By default this is 1000.
* `-o` causes flickrate to open the listed photos in a browser window,
in addition to lising the URLs.

## Platform Support
For operations that require opening a browser, flickrate supports Windows 10
and Mac, and maybe Linux. (My test environment for Linux is not useful for
this.  In theory the code works in a gnome environment.  Please let me know!)
Things that require a browser are the `-o` flag and the `-user` flag.
If you aren't using either of those,
flickrate should run proplerly on any platform that supports go.
