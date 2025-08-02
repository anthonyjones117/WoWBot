import { Component, OnInit } from '@angular/core';
import { SocketService } from './socket.service';
import { ActivatedRoute, Router } from '@angular/router';
import { HttpClient } from '@angular/common/http';
import { environment } from 'src/environments/environment';

interface Message {
  sender: string;
  content: string;
}

@Component({
  selector: 'app-root',
  templateUrl: './app.component.html',
  styleUrls: ['./app.component.css'],
})
export class AppComponent implements OnInit {
  chatBox = '';
  messages: Message[] = [];

  username: string = 'anthony';
  password: string = '';
  loggedIn: boolean = false;
  //loggedIn: boolean = true;
  loginError: boolean = false;
  profileInfo: any = null;

  constructor(
  private socketService: SocketService,
  private route: ActivatedRoute,
  private router: Router,
  private http: HttpClient
) {}

 ngOnInit() {

  const savedLogin = localStorage.getItem('loggedIn');
  const savedUsername = localStorage.getItem('username');
  if (savedLogin === 'true' && savedUsername) {
    this.loggedIn = true;
    this.username = savedUsername;
  }

  console.log('Full URL:', window.location.href);
  console.log('Raw query string:', window.location.search);
  this.route.queryParams.subscribe(params => {
  const code = params['code'];
  console.log('OAuth code from queryParams:', code);

  if (code) {
    this.http.get<any>(`http://localhost:12345/exchange?code=${code}`).subscribe({
      next: (token: any) => {
        console.log('Blizzard token response:', token);
        localStorage.setItem('blizz_token', JSON.stringify(token));
        this.router.navigate(['/']);
      },
      error: (err: any) => console.error('OAuth exchange failed:', err),
    });
  }
});


  // Setup WebSocket
  this.socketService.getEventListener().subscribe((event) => {
    if (event.type === 'message') {
      const data = event.data;
      this.messages.push({
        sender: data.sender || 'System',
        content: data.content,
      });
    } else if (event.type === 'open') {
      console.log('WebSocket connected');
    } else if (event.type === 'close') {
      console.log('WebSocket disconnected');
    }
  });
}

login() {
  if (this.password === ' ' && this.username.trim() !== '') {
    this.loggedIn = true;
    this.loginError = false;
    localStorage.setItem('username', this.username);
    localStorage.setItem('loggedIn', 'true');
  } else {
    this.loginError = true;
  }
}

logout() {
  this.loggedIn = false;
  this.username = '';
  this.password = '';
  this.chatBox = '';
  this.messages = [];
  localStorage.removeItem('username');
  localStorage.removeItem('loggedIn');
}

  send() {
    if (this.chatBox.trim() !== '') {
      const msg: Message = {
        sender: this.username,
        content: this.chatBox.trim(),
      };
      this.socketService.send(JSON.stringify(msg));
      this.chatBox = '';
    }
  }

isSystemMessage(message: any): string {
  const isAgent = message.sender === 'Agent';

  if (!isAgent) {
    // Regular user message
    return `<strong>${message.sender}:</strong> ${this.escapeHtml(message.content)}`;
  }

  // Agent message formatting with bold labels
  const formatted = message.content
    .split('\n')
    .filter((line: string) => line.trim() !== '') // skip empty lines
    .map((line: string) => {
      const [label, ...rest] = line.split(':');
      const value = rest.join(':'); // in case the value also contains colons
      if (value) {
        return `<div><strong>${this.escapeHtml(label.trim())}:</strong> ${this.escapeHtml(value.trim())}</div>`;
      }
      return `<div>${this.escapeHtml(line.trim())}</div>`;
    })
    .join('');

  return `
    <div style="
      background: #f1f1f1;
      border-radius: 8px;
      padding: 0.8em;
      margin: 0.5em 0;
      border: 1px solid #ccc;
    ">
      <strong>Agent:</strong><br />
      ${formatted}
    </div>
  `;
}

// Helper to prevent unsafe characters from breaking HTML
escapeHtml(text: string): string {
  return text.replace(/&/g, '&amp;')
             .replace(/</g, '&lt;')
             .replace(/>/g, '&gt;')
             .replace(/"/g, '&quot;')
             .replace(/'/g, '&#039;');
}

connectWithBlizzard() {
  const clientId = environment.client_id; // replace with your real Blizzard client ID
  const redirectUri = 'http://localhost:4200'; // should match what's in your Blizzard app setup
  const region = 'us'; // or 'eu', 'kr', 'tw'
  const state = '1234';  // Or any unique string

  const authUrl = `https://${region}.battle.net/oauth/authorize` +
                  `?client_id=${clientId}` +
                  `&scope=wow.profile` +
                  `&redirect_uri=${encodeURIComponent(redirectUri)}` +
                  `&response_type=code`+
                  `&state=${state}`;
  window.location.href = authUrl;
}

fetchCharacterProfile() {
  const tokenData = localStorage.getItem('blizz_token');
  console.log("Raw token from localStorage:", tokenData);
  if (!tokenData) return;

  const token = JSON.parse(tokenData).access_token;

  this.http.get<any>('http://localhost:12345/profile', {
    headers: { Authorization: `Bearer ${token}` }
  }).subscribe({
    next: (data: any) => {
      console.log("Profile data:", data);
      this.profileInfo = data;

      //  Find Bodicea
      const allCharacters = data.wow_accounts?.[0]?.characters || [];
      const main = allCharacters.find((c: any) => c.name.toLowerCase() === "bodicea");
      if (!main) {
        console.warn("Bodicea not found!");
        return;
      }

      const simplified = {
        name: main.name,
        race: main.playable_race?.name?.en_US || "Unknown",
        class: main.playable_class?.name?.en_US || "Unknown",
        realm: main.realm?.slug?.toLowerCase() || "Unknown", 
        token: token
      };
      console.log("Saving main character:", simplified);

      // 🔵 Save to backend
      this.http.post(`http://localhost:12345/save-character?user=${this.username}`, simplified).subscribe({
        next: () => console.log("Saved to memory.json!"),
        error: (err: any) => console.error("Save failed:", err)
      });
    },
    error: (err: any) => {
      console.error("Blizzard API error:", err);
    }
  });
}



}
