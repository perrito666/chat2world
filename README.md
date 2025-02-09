# chat2world

A microblogging tool that proxies IM to various microblogging and archiving solutions

## Progress so far

### **All Is Untested**

## DO NOT USE YET

* It stores credentials in a plaintext file
* It requires some secrets in the environment
* I need to review logs to ensure no secret is being logged.

### IM Support

So far we support telegram as it was trivial to make a bot for it, ideally I would like to implement at least signal.

### Microblogging Support

* Mastodon support is there, you can post to mastodon from telegram Text and Images including Alt-text

## Future

### IM Support

I want signal, but I did not yet find clear docs on how to do this in go

### Microblogging Support

* Next in line is bluesky, the whole motivation for this project is to be able to post to bluesky AND mastodon in one go
* I consider Twitter, but I do not think the hassle is worth it, Twitter does not want to be used outside the official client, let it be.
* Finally, I would like to be able to produce daily blogposts in hugo with all the day toots.



