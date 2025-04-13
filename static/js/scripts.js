// Wait for document to be ready
$(document).ready(function() {
    console.log('Go on Airplanes app is ready!');
    
    // Add the hover-scale class to all cards
    $('.bg-white.shadow').addClass('hover-scale');
    
    // Simple click handler for demonstration
    $('a').on('click', function() {
        console.log('Link clicked:', $(this).attr('href'));
    });
}); 