// A C program to demonstrate zombie process.
// Child becomes zombie as parent is sleeping
// when child process exits.
#include <stdlib.h>
#include <sys/types.h>
#include <unistd.h>

int main ()
{
	// Create a child process
	int child_pid = fork();

	// Sleep the parent process
	if (child_pid > 0) {
		sleep (60);
	}
	// Exit the child process
	else {
		exit (0);
	}
	return 0;
}