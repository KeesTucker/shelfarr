book management app.

<img width="2558" height="1302" alt="image" src="https://github.com/user-attachments/assets/48b3e314-291b-4abb-8cb5-765ef45c76bf" />
<img width="2556" height="1295" alt="image" src="https://github.com/user-attachments/assets/70439cc4-1f04-4440-9d26-3341c57c1ccb" />
<img width="2555" height="1243" alt="image" src="https://github.com/user-attachments/assets/d5e59ccc-7596-40f9-824d-d9522ba5610f" />


born out of frustrations with readarr/lazylibrarian/audiobookrequest.

a lot of ai generated shit. I do not like ai. I am a hypocrite. 

I would not use this until further notice, it's probably full of heinous security issues. If you do use it, at least put it behind zero trust.

eventually this will be properly looked over and built without ai, I just wanted to see how far I'd get vibe coding this in a couple afternoons after work for my girlfriend.

I am a little concerned with how well it's gone so far.

I do not endorse using this for piracy.


stack:
- backend; go, sqlite
- frontend; svelte-kit, ts, vite, shadcn/ui, tailwindcss


integration:
- audiobookshelf for credential checking (req)
- prowlarr (req)
- qbit (req)
- discord webhooks

auth:
- http only secure cookie


attributions:
- books by Contributor Icons from <a href="https://thenounproject.com/browse/icons/term/books/" target="_blank" title="books Icons">Noun Project</a> (CC BY 3.0)
