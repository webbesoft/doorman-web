package handlers

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"github.com/tmunongo/doorman-go/models"
)

type Handler struct {
	DB *gorm.DB
}

// Track handles incoming analytics data
func (h *Handler) Track(c echo.Context) error {
	var req struct {
		URL      string `json:"url"`
		Referrer string `json:"referrer"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	// Hash IP for GDPR compliance (no personal data stored)
	ip := c.RealIP()
	hasher := sha256.New()
	hasher.Write([]byte(ip))
	ipHash := fmt.Sprintf("%x", hasher.Sum(nil))

	pageView := models.PageView{
		URL:       req.URL,
		Referrer:  req.Referrer,
		UserAgent: c.Request().UserAgent(),
		IPHash:    ipHash,
		CreatedAt: time.Now(),
	}

	if err := h.DB.Create(&pageView).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to save data"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})

}

// ServeTracker serves the tracking JavaScript
func (h *Handler) ServeTracker(c echo.Context) error {
	script := `
	(function() {
		function getTrackerURL() {
			var cs = document.currentScript;
			if (cs && cs.dataset && cs.dataset.endpoint) return cs.dataset.endpoint;
			if (cs && cs.src) {
			var u = new URL(cs.src);
				return u.origin + '/event';
			}
			return '/event';
		}

		var TRACK_URL = getTrackerURL();

		// GDPR compliant tracker - no cookies, no personal data
		function track() {
			fetch(TRACK_URL, {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
				},
				body: JSON.stringify({
					url: window.location.href,
					referrer: document.referrer
				}),
				keepalive: true
			}).catch(function() {
				// Silently fail
			});
		}

		// Track initial page load
		if (document.readyState === 'complete') {
			track();
		} else {
			window.addEventListener('load', track);
		}

		// Track navigation for SPAs
		var originalPushState = history.pushState;
		var originalReplaceState = history.replaceState;

		history.pushState = function() {
			originalPushState.apply(history, arguments);
			setTimeout(track, 100);
		};

		history.replaceState = function() {
			originalReplaceState.apply(history, arguments);
			setTimeout(track, 100);
		};

		window.addEventListener('popstate', function() {
			setTimeout(track, 100);
		});

	})();`

	c.Response().Header().Set("Content-Type", "application/javascript")
	c.Response().Header().Set("Cache-Control", "public, max-age=3600")
	return c.String(http.StatusOK, script)

}

// LoginPage renders the login page
func (h *Handler) LoginPage(c echo.Context) error {
	html := `<!DOCTYPE html>

	<html lang="en">
	<head>
		<meta charset="UTF-8">
		<meta name="viewport" content="width=device-width, initial-scale=1.0">
		<title>Doorman Analytics - Login</title>
		<script src="https://cdn.tailwindcss.com"></script>
	</head>
	<body class="bg-gray-100 min-h-screen flex items-center justify-center">
		<div class="bg-white p-8 rounded-lg shadow-md w-96">
			<h1 class="text-2xl font-bold mb-6 text-center">Doorman Login</h1>
			<form action="/login" method="post">
				<div class="mb-4">
					<label class="block text-gray-700 text-sm font-bold mb-2">Username</label>
					<input type="text" name="username" required 
						class="w-full px-3 py-2 border rounded-lg focus:outline-none focus:border-blue-500">
				</div>
				<div class="mb-6">
					<label class="block text-gray-700 text-sm font-bold mb-2">Password</label>
					<input type="password" name="password" required
						class="w-full px-3 py-2 border rounded-lg focus:outline-none focus:border-blue-500">
				</div>
				<button type="submit" 
						class="w-full bg-blue-500 text-white py-2 px-4 rounded-lg hover:bg-blue-600 transition duration-200">
					Login
				</button>
			</form>
		</div>
	</body>
	</html>`
	return c.HTML(http.StatusOK, html)
}

// Login handles user authentication
func (h *Handler) Login(c echo.Context) error {
	username := c.FormValue("username")
	password := c.FormValue("password")

	var user models.User
	if err := h.DB.Where("username = ?", username).First(&user).Error; err != nil {
		return c.Redirect(http.StatusFound, "/login?error=invalid")
	}

	if !models.CheckPasswordHash(password, user.Password) {
		return c.Redirect(http.StatusFound, "/login?error=invalid")
	}

	sess, _ := session.Get("session", c)
	sess.Values["user_id"] = user.ID
	sess.Save(c.Request(), c.Response())

	return c.Redirect(http.StatusFound, "/dashboard")

}

// Logout handles user logout
func (h *Handler) Logout(c echo.Context) error {
	sess, _ := session.Get("session", c)
	sess.Values = make(map[interface{}]interface{})
	sess.Save(c.Request(), c.Response())
	return c.Redirect(http.StatusFound, "/login")
}

// Dashboard renders the analytics dashboard
func (h *Handler) Dashboard(c echo.Context) error {
	// Get total page views
	var totalViews int64
	h.DB.Model(&models.PageView{}).Count(&totalViews)

	// Get unique visitors (unique IP hashes)
	var uniqueVisitors int64
	h.DB.Model(&models.PageView{}).Distinct("ip_hash").Count(&uniqueVisitors)

	// Get top pages
	var topPages []struct {
		URL   string
		Count int64
	}
	h.DB.Model(&models.PageView{}).
		Select("url, COUNT(*) as count").
		Group("url").
		Order("count DESC").
		Limit(10).
		Scan(&topPages)

	// Get top referrers
	var topReferrers []struct {
		Referrer string
		Count    int64
	}
	h.DB.Model(&models.PageView{}).
		Select("referrer, COUNT(*) as count").
		Where("referrer != ''").
		Group("referrer").
		Order("count DESC").
		Limit(10).
		Scan(&topReferrers)

	// Get recent views for chart (last 7 days)
	var dailyViews []struct {
		Date  string
		Count int64
	}
	h.DB.Model(&models.PageView{}).
		Select("DATE(created_at) as date, COUNT(*) as count").
		Where("created_at >= ?", time.Now().AddDate(0, 0, -7)).
		Group("DATE(created_at)").
		Order("date").
		Scan(&dailyViews)

	html := fmt.Sprintf(`<!DOCTYPE html>

<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>Doorman Dashboard</title>
	<script src="https://cdn.tailwindcss.com"></script>
	<script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
</head>
<body class="bg-gray-100 min-h-screen">
	<nav class="bg-white shadow-sm border-b">
		<div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
			<div class="flex justify-between h-16">
				<div class="flex items-center">
					<h1 class="text-xl font-semibold">Doorman Dashboard</h1>
				</div>
				<div class="flex items-center">
					<form action="/logout" method="post" class="inline">
						<button type="submit" class="text-gray-500 hover:text-gray-700">Logout</button>
					</form>
				</div>
			</div>
		</div>
	</nav>

    <main class="max-w-7xl mx-auto py-6 sm:px-6 lg:px-8">
    	<!-- Stats Overview -->
    	<div class="grid grid-cols-1 md:grid-cols-3 gap-6 mb-8">
    		<div class="bg-white p-6 rounded-lg shadow">
    			<div class="text-2xl font-bold text-blue-600">%d</div>
    			<div class="text-gray-600">Total Page Views</div>
    		</div>
    		<div class="bg-white p-6 rounded-lg shadow">
    			<div class="text-2xl font-bold text-green-600">%d</div>
    			<div class="text-gray-600">Unique Visitors</div>
    		</div>
    		<div class="bg-white p-6 rounded-lg shadow">
    			<div class="text-2xl font-bold text-purple-600">%.1f%%</div>
    			<div class="text-gray-600">Return Rate</div>
    		</div>
    	</div>

    	<div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
    		<!-- Chart -->
    		<div class="bg-white p-6 rounded-lg shadow">
    			<h3 class="text-lg font-semibold mb-4">Page Views (Last 7 Days)</h3>
    			<canvas id="viewsChart" width="400" height="200"></canvas>
    		</div>

    		<!-- Top Pages -->
    		<div class="bg-white p-6 rounded-lg shadow">
    			<h3 class="text-lg font-semibold mb-4">Top Pages</h3>
    			<div class="space-y-2">`, totalViews, uniqueVisitors, float64(totalViews-uniqueVisitors)/float64(totalViews)*100)

	for _, page := range topPages {
		html += fmt.Sprintf(`
    				<div class="flex justify-between items-center py-2 border-b">
    					<span class="text-sm truncate flex-1 mr-4">%s</span>
    					<span class="text-sm font-medium text-gray-600">%d</span>
    				</div>`, page.URL, page.Count)
	}

	html += `
    			</div>
    		</div>
    	</div>

    	<!-- Top Referrers -->
    	<div class="mt-6">
    		<div class="bg-white p-6 rounded-lg shadow">
    			<h3 class="text-lg font-semibold mb-4">Top Referrers</h3>
    			<div class="grid grid-cols-1 md:grid-cols-2 gap-4">`

	for _, ref := range topReferrers {
		html += fmt.Sprintf(`
    				<div class="flex justify-between items-center py-2 border-b">
    					<span class="text-sm truncate flex-1 mr-4">%s</span>
    					<span class="text-sm font-medium text-gray-600">%d</span>
    				</div>`, ref.Referrer, ref.Count)
	}

	html += `
    			</div>
    		</div>
    	</div>
    </main>

    <script>
    	// Chart.js setup
    	const ctx = document.getElementById('viewsChart').getContext('2d');
    	const chart = new Chart(ctx, {
    		type: 'line',
    		data: {
    			labels: [`

	for i, view := range dailyViews {
		if i > 0 {
			html += ", "
		}
		html += fmt.Sprintf("'%s'", view.Date)
	}

	html += `],
    			datasets: [{
    				label: 'Page Views',
    				data: [`

	for i, view := range dailyViews {
		if i > 0 {
			html += ", "
		}
		html += strconv.FormatInt(view.Count, 10)
	}

	html += `],
    				borderColor: 'rgb(59, 130, 246)',
    				backgroundColor: 'rgba(59, 130, 246, 0.1)',
    				tension: 0.1
    			}]
    		},
    		options: {
    			responsive: true,
    			scales: {
    				y: {
    					beginAtZero: true
    				}
    			}
    		}
    	});
    </script>

</body>
</html>`

	return c.HTML(http.StatusOK, html)

}
