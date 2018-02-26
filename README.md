# flickrate

flickrate is a different way to look at the statistics for you flickr photos.
On their statistics page, flickr tracks the top ten photos in terms of all time cumulative
views.  But after a while, this list rarely changes, because older photos keep accumulating
views and newer ones can never catch up.  I was interested in which of my more recent
photos were doing well, so I wrote flickrate.  This program will show you which of your
photos are getting more views per day, which should allow recent photos a chance.

flickrate is a command line application.  If you're not familiar with the
command line, I'm afraid you're not going to find it terribly easy to use.  If
the shell prompt doesn't intimidate you, then read on!

To use flickrate, you'll need an API key and secret from flickr.  You can get them
[here](https://www.flickr.com/services/apps/create/noncommercial/?).  The first time
you use it, you'll have to supply these to flickrate using the `-key` and `-secret`
parameters.  After that, flickrate will remember them, so you don't have to
type them every time.  If you want to check photos that are private, or that
are not visible in safe mode, you'll need to authorize flickrate.  Do this by
providing the `-user` parameter.  Again, flickrate will remember the login,
so you don't have to do it every time.

Once all of that red tape is out of the way, simply run flickrate with the
name of the user whose stats you want to check.  If you've authorized
flickrate with the `-user` parameter, then by default your own photos
will be checked.
