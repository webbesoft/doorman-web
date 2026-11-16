// Keep the numerical alternative one click away from the chart.
const dailyRecords = document.getElementById('daily-records');
function revealDailyRecords() {
  if (dailyRecords && window.location.hash === '#daily-records') {
    dailyRecords.open = true;
  }
}
document.querySelectorAll('a[href="#daily-records"]').forEach(link => {
  link.addEventListener('click', () => {
    if (dailyRecords) dailyRecords.open = true;
  });
});
window.addEventListener('hashchange', revealDailyRecords);
revealDailyRecords();
