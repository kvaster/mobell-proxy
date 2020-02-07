# Mobell Proxy

Mobell proxy is a server companion for [mobell](https://github.com/kvaster/mobell) android application.

Mobell android app is working quit well, but there are two problems:

* Video start is very slow, sometimes it takes more then three seconds.
* Ring will not stop on other mobell instances when using more then one mobell app.

Mobell proxy will cache video buffer from last key frame - this allows video to start immediatelly.
Also mobell proxy will send proper events to all connected mobell applications.

# License

Copyright 2020 Viktor Kuzmin

This copy of Mobell proxy is licensed under the
Apache (Software) License, version 2.0 ("the License").
See the License for details about distribution rights, and the
specific rights regarding derivate works.

You may obtain a copy of the License at:

http://www.apache.org/licenses/LICENSE-2.0
