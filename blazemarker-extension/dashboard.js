// --- Daily background photo ---
async function setDailyBackground() {
  const today = new Date().toDateString();
  let storedData = JSON.parse(localStorage.getItem('dailyPhoto') || '{}');
  if (storedData.date !== today) {
    let photoUrl;
    try {
      const res = await fetch('http://localhost:3000/api/photos/random');
      const data = await res.json();
      photoUrl = data.url;
    } catch (e) {
      // Fallback to hardcoded if API fails
      const fallbackUrls = [
        'http://localhost:3000/photos/galleries/2004-04/April2004Digital_005.jpg',
        'http://localhost:3000/photos/galleries/2004-06/June2004Digital_027.jpg',
        'http://localhost:3000/photos/galleries/2004-04/April2004Digital_009.jpg'
      ];
      photoUrl = fallbackUrls[Math.floor(Math.random() * fallbackUrls.length)];
    }
    storedData = { date: today, url: photoUrl };
    localStorage.setItem('dailyPhoto', JSON.stringify(storedData));
  }
  document.body.style.backgroundImage = `url('${storedData.url}')`;
}
setDailyBackground();

// --- Blazemarker Basic Auth ---
async function blazemarkerAuthFetch(url, options = {}) {
  let username = localStorage.getItem('blazemarkerUsername');
  let password = sessionStorage.getItem('blazemarkerPassword');
  // Try fetch without credentials first
  let resp = await fetch(url, { ...options });
  if (resp.status === 401 || resp.status === 403) {
    // Not authenticated, prompt for credentials
    if (!username) username = prompt('Blazemarker username:');
    if (!password) password = prompt('Blazemarker password:');
    if (username) localStorage.setItem('blazemarkerUsername', username);
    if (password) sessionStorage.setItem('blazemarkerPassword', password);
    options.headers = options.headers || {};
    options.headers['Authorization'] = 'Basic ' + btoa(username + ':' + password);
    resp = await fetch(url, options);
  }
  return resp;
}

// --- Personalized greeting and time ---
const greeting = document.getElementById('greeting');
const timeDiv = document.getElementById('time');
let userName = localStorage.getItem('blazemarkerUsername') || 'User';
blazemarkerAuthFetch('http://localhost:3000/api/users/online')
  .then(r => r.json())
  .then(users => {
    // Find current user in response
    const current = users.find(u => u.is_current_user);
    if (current) userName = current.handle || current.username;
    updateGreeting();
  })
  .catch(() => updateGreeting());

function updateGreeting() {
  const hour = new Date().getHours();
  let greet = 'Good morning';
  if (hour >= 12 && hour < 18) greet = 'Good afternoon';
  else if (hour >= 18) greet = 'Good evening';
  greeting.textContent = `${greet}, ${userName}.`;
  timeDiv.textContent = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}
updateGreeting();
setInterval(updateGreeting, 60000);


// --- Recent articles (from JSON API) ---
fetch('http://localhost:3000/api/articles/recent')
  .then(r => r.json())
  .then(articles => {
    const list = document.getElementById('article-list');
    list.innerHTML = '';
    articles.forEach(a => {
      const li = document.createElement('li');
      const date = a.date ? new Date(a.date).toLocaleDateString() : '';
      li.textContent = `${date} - ${a.title}, by ${a.author} `;
      const link = document.createElement('a');
      link.href = `https://blazemarker.com/article/view/${a.id}`;
      link.textContent = '[view]';
      link.target = '_blank';
      li.appendChild(link);
      list.appendChild(li);
    });
  });

// Show/hide article list with floating button
const showArticlesBtn = document.getElementById('show-articles-btn');
const articleListContainer = document.getElementById('article-list-container');
let articlesVisible = false;
showArticlesBtn.onclick = () => {
  articlesVisible = !articlesVisible;
  articleListContainer.style.display = articlesVisible ? 'block' : 'none';
};

// --- Online users ---
blazemarkerAuthFetch('http://localhost:3000/api/users/online')
  .then(r => r.json())
  .then(users => {
    const list = document.getElementById('user-list');
    list.innerHTML = '';
    // Filter for online users who are not the current user
    const onlineUsers = users.filter(u => u.is_online && !u.is_current_user);
    if (onlineUsers.length === 0) {
      list.innerHTML = '<li>No other users online.</li>';
      return;
    }
    onlineUsers.forEach(u => {
      const li = document.createElement('li');
      const btn = document.createElement('button');
      btn.textContent = u.handle || u.username;
      btn.onclick = () => {
        window.open(`https://blazemarker.com/chat?with=${u.username}`, '_blank');
      };
      li.appendChild(btn);
      list.appendChild(li);
    });
  })
  .catch(err => {
    const list = document.getElementById('user-list');
    list.innerHTML = `<li>Error loading users: ${err.message}</li>`;
    console.error('User list fetch error:', err);
  });

// --- Todos (server-backed, per-user) ---
const todoList = document.getElementById('todo-list');
const newTodo = document.getElementById('new-todo');
const addTodo = document.getElementById('add-todo');
const focusInput = document.getElementById('daily-focus');
focusInput.value = localStorage.getItem('dailyFocus') || '';
focusInput.onblur = () => localStorage.setItem('dailyFocus', focusInput.value);

async function renderTodos() {
  try {
    const res = await blazemarkerAuthFetch('http://localhost:3000/api/todos');
    if (!res.ok) throw new Error('Failed to load todos');
    const todos = await res.json();
    todoList.innerHTML = '';
    if (!todos || todos.length === 0) {
      todoList.innerHTML = '<li>No todos</li>';
      return;
    }
    todos.forEach(t => {
      const li = document.createElement('li');
      li.style.display = 'flex';
      li.style.justifyContent = 'space-between';
      li.style.alignItems = 'center';
      li.style.padding = '6px 4px';

      const span = document.createElement('span');
      span.textContent = t.text;

      const delBtn = document.createElement('button');
      delBtn.className = 'todo-delete';
      delBtn.title = 'Delete todo';
      delBtn.innerHTML = '&#10005;';
      delBtn.onclick = async (ev) => {
        ev.stopPropagation();
        const ok = confirm('Delete this todo?');
        if (!ok) return;
        try {
          const del = await blazemarkerAuthFetch(`http://localhost:3000/api/todos?id=${t.ID || t.id}`, { method: 'DELETE' });
          if (!del.ok) throw new Error('Delete failed');
          renderTodos();
        } catch (err) {
          console.error('Delete todo failed', err);
          alert('Failed to delete todo');
        }
      };

      li.appendChild(span);
      li.appendChild(delBtn);
      todoList.appendChild(li);
    });
  } catch (err) {
    todoList.innerHTML = `<li>Error loading todos: ${err.message}</li>`;
  }
}

addTodo.onclick = async () => {
  const val = newTodo.value.trim();
  if (!val) return;
  try {
    const res = await blazemarkerAuthFetch('http://localhost:3000/api/todos', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ text: val })
    });
    if (!res.ok) throw new Error('Failed to add todo');
    newTodo.value = '';
    renderTodos();
  } catch (err) {
    console.error('Add todo failed', err);
  }
};
renderTodos();

// --- SEPTA Next Trains ---
async function fetchSeptaTrains(origin, dest, listId, directionLabel) {
  const cacheKey = `septa_${origin}_${dest}`;
  const cached = localStorage.getItem(cacheKey);
  const now = Date.now();

  if (cached) {
    const data = JSON.parse(cached);
    if (now - data.timestamp < 300000) { // 5 min cache
      renderSeptaTrains(data.trains, listId, directionLabel);
      return;
    }
  }

  try {
    //https://www3.septa.org/api/NextToArrive/index.php?req1=Chestnut%20Hill%20West&req2=Suburban%20Station&req3=3

    const url = `http://localhost:3000/api/septa/nexttrains?origin=${encodeURIComponent(origin)}&dest=${encodeURIComponent(dest)}`;
    const res = await fetch(url);
    if (!res.ok) throw new Error('SEPTA API error');
    const trains = await res.json();

    localStorage.setItem(cacheKey, JSON.stringify({ trains, timestamp: now }));
    renderSeptaTrains(trains, listId, directionLabel);
  } catch (err) {
    console.error('SEPTA fetch failed:', err);
    document.getElementById(listId).innerHTML = '<li>Service info unavailable</li>';
  }
}

function renderSeptaTrains(trains, listId, directionLabel) {
  const list = document.getElementById(listId);
  list.innerHTML = '';
  if (!trains || trains.length === 0) {
    list.innerHTML = '<li>No upcoming trains</li>';
    return;
  }
  trains.forEach(t => {
    const li = document.createElement('li');
    li.style.marginBottom = '6px';
    const delay = t.orig_delay === 'On time' ? '' : ` (${t.orig_delay})`;
    li.innerHTML = `Train ${t.orig_train}: Departs ${t.orig_departure_time}, Arrives ${t.orig_arrival_time}${delay}`;
    list.appendChild(li);
  });
}

// Fetch both directions (run on load, and optionally setInterval every 5-10 min)
fetchSeptaTrains('Chestnut Hill West', 'Suburban Station', 'septa-to-suburban', 'To Center City');
fetchSeptaTrains('Suburban Station', 'Chestnut Hill West', 'septa-to-chw', 'To Chestnut Hill West');

// Optional: Refresh every 5 minutes
setInterval(() => {
  fetchSeptaTrains('Chestnut Hill West', 'Suburban Station', 'septa-to-suburban', 'To Center City');
  fetchSeptaTrains('Suburban Station', 'Chestnut Hill West', 'septa-to-chw', 'To Chestnut Hill West');
}, 300000);
