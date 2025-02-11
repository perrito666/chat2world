# chat2world

A microblogging tool that proxies IM to various microblogging and archiving solutions

## Before you use this (this is about security)

* Credentials for telegram and mastodon are stored in files on te machine, encrypted, but still.
* It requires one secret in the environment,the encryption key.
* I need to review logs to ensure no secret is being logged.

## Progress so far

### **All Is Untested**

### IM Support

So far we support telegram as it was trivial to make a bot for it, ideally I would like to implement at least signal.

### Microblogging Support

* Mastodon support is there, you can post to mastodon from telegram Text and Images including Alt-text
* Bluesky support is there, you can post to bluesky from telegram Text and Images including Alt-text, long posts will be split in 300 chars chunks without breaking words (if you try the clever longer than 300 chars word it will just beak)

## Future

### IM Support

I want signal, but I did not yet find clear docs on how to do this in go

### Microblogging Support

* I consider Twitter, but I do not think the hassle is worth it, Twitter does not want to be used outside the official client, let it be.
* Finally, I would like to be able to produce daily blogposts in hugo with all the day toots.

----

# Usage

## Setup

In its core, this is a telegram bot, I encourage to read [their docs](https://core.telegram.org/bots/api) and
[the doc of the library I use](github.com/go-telegram/bot).

To run, chat2world requires a few things:
* A Public URL you can listen into.
* The public URL of the Telegram webhook that will be listening.
* A Telegram Bot Token.
* A Telegram Webhook Secret.
* A nice encryption password

Ideally this should run out of an encrypted `telegram.config` file which needs to be generated.
The file, in clear format, should look like this:

```json
{
"CHAT2WORLD_URL": "https://your.url.here",
"TELEGRAM_BOT_TOKEN": "1234567890:yourtokenhere",
"TELEGRAM_WEBHOOK_SECRET": "yoursecrethere",
"TELEGRAM_LISTEN_ADDR": ":8077"
}
```

The encryption password should be stored in the environment as `CHAT2WORLD_PASSWORD`.

You can create the encrypted config one of two ways:

1. Create and filll the `telegram.config` file, then run `CHAT2WORLD_PASSWORD='foobar' ./chat2world --encrypt-file telegram.config` which will create a `telegram.config.enc` file, rename that to `telegram.config` and delete the original.
2. Set all the values in the environment and run `CHAT2WORLD_PASSWORD='foobar' ./chat2world` making sure the `telegram.config` file is not present and it should create it.

## Running

Once you have the `telegram.config` file, you can run the bot with `CHAT2WORLD_PASSWORD='foobar' ./chat2world --with-allowed-telegram-user=<youruserid>` 
(you can figure out your user id by asking [@userinfobot](https://telegram.me/userinfobot) ).

The `--with-allowed-telegram-user=` flag is important as it determines which users can use your bot as a client, you can specify as many as you want by just repeating the flag. 

## Connecting Mastodon

Start a chat with your bot (you could do this in public as it will use your userID not your chatID)
and issue the `/mastodon_auth` command (this is necessary only once, it will store the token in an encrypted file named `<userID>.json`).

The whole auth process is interactive, it will ask you to open a URL in your browser, login and paste the code back in the chat.

## Connecting Bluesky

Start a chat with your bot (you could do this in public as it will use your userID not your chatID)
and issue the `/bluesky_auth` command (this is necessary only once, it will store the identifier and app password in an encrypted file named `<userID>.bsky.json`).

Bear in mind, this uses an **APP PASSWORD** not your main password, you can generate one in the settings of your bluesky account.


## Posting

To begin a post you need to issue the `/new [lang=es | es]` command, this will set the bot ready for your inputs.

Any input that is not a known command while in post mode will be considered part of the post.

You can also send images, if you add a caption to them, it will be used as alt-text in mastodon.

Finally, you can either `/send` or `/cancel` the post.

## Tooling

There are flags provided for encryption and decryption of files.
Find them with `./chat2world --help` (they also require the secret in the environment)
